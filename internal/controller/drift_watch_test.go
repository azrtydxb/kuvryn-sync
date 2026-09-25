/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/config"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1alpha1 "github.com/azrtydxb/kuvryn-sync/api/v1alpha1"
	"github.com/azrtydxb/kuvryn-sync/internal/impersonate"
	"github.com/azrtydxb/kuvryn-sync/internal/renderer"
)

// pathRenderer renders a fixed set of objects per Application source path.
type pathRenderer map[string][]unstructured.Unstructured

func (p pathRenderer) Render(_ context.Context, input renderer.Input) ([]unstructured.Unstructured, error) {
	objects := make([]unstructured.Unstructured, 0, len(p[input.Path]))
	for _, obj := range p[input.Path] {
		objects = append(objects, *obj.DeepCopy())
	}
	return objects, nil
}

// The controller role here is the generated one plus list/watch on Widgets
// only: Widget drift must be noticed through a watch, Gizmo drift through the
// periodic resync.
var _ = Describe("Drift detection for custom kinds", Ordered, func() {
	const (
		controllerAccount = "kuvryn-sync-test-drift"
		resync            = 8 * time.Second
	)
	var stop context.CancelFunc
	watchedApp := types.NamespacedName{Name: "watched-app", Namespace: "default"}
	resyncedApp := types.NamespacedName{Name: "resynced-app", Namespace: "default"}

	BeforeAll(func() {
		ensureNamespace(ctx, "payments")
		ensureCustomKind(ctx, "Widget", "widgets")
		ensureCustomKind(ctx, "Gizmo", "gizmos")

		role := generatedManagerRole()
		role.Name = controllerAccount
		role.Rules = append(role.Rules, rbacv1.PolicyRule{APIGroups: []string{"example.com"}, Resources: []string{"widgets"}, Verbs: []string{"list", "watch"}})
		Expect(k8sClient.Create(ctx, role)).To(Succeed())
		Expect(k8sClient.Create(ctx, &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: controllerAccount, Namespace: "default"}})).To(Succeed())
		Expect(k8sClient.Create(ctx, &rbacv1.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: controllerAccount},
			RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: controllerAccount},
			Subjects:   []rbacv1.Subject{{Kind: rbacv1.ServiceAccountKind, Name: controllerAccount, Namespace: "default"}},
		})).To(Succeed())
		createRepository(ctx)

		controllerConfig := serviceAccountConfig(controllerAccount)
		mgr, err := ctrl.NewManager(controllerConfig, ctrl.Options{
			Scheme:                 k8sClient.Scheme(),
			Metrics:                metricsserver.Options{BindAddress: "0"},
			HealthProbeBindAddress: "0",
			Controller:             config.Controller{SkipNameValidation: ptr.To(true)},
			Client:                 client.Options{Cache: &client.CacheOptions{DisableFor: []client.Object{&corev1.Secret{}}}},
		})
		Expect(err).NotTo(HaveOccurred())
		render := pathRenderer{
			"apps/watched":  {customObject("Widget", "watched-widget", "desired")},
			"apps/resynced": {customObject("Gizmo", "resynced-gizmo", "desired")},
		}
		Expect((&ApplicationReconciler{
			Client:         mgr.GetClient(),
			Scheme:         mgr.GetScheme(),
			SourceResolver: fakeResolver{},
			Renderers: func(corev1alpha1.RenderType) (renderer.Renderer, error) {
				return render, nil
			},
			Impersonation:         impersonate.New(controllerConfig, client.Options{Scheme: mgr.GetScheme()}),
			DefaultServiceAccount: testServiceAccount,
			DriftResyncInterval:   resync,
		}).SetupWithManager(mgr)).To(Succeed())

		var managerCtx context.Context
		managerCtx, stop = context.WithCancel(ctx)
		go func() {
			defer GinkgoRecover()
			Expect(mgr.Start(managerCtx)).To(Succeed())
		}()

		for key, path := range map[types.NamespacedName]string{watchedApp: "apps/watched", resyncedApp: "apps/resynced"} {
			app := newApplication(key.Name, corev1alpha1.RenderTypeYAML)
			app.Spec.Source.Path = path
			app.Spec.Sync.Automatic = true
			Expect(k8sClient.Create(ctx, app)).To(Succeed())
		}
	})

	AfterAll(func() {
		stop()
		for _, key := range []types.NamespacedName{watchedApp, resyncedApp} {
			deleteObject(ctx, &corev1alpha1.Application{ObjectMeta: metav1.ObjectMeta{Name: key.Name, Namespace: key.Namespace}})
			deleteApplicationRevisions(ctx, key.Name)
		}
		for _, obj := range []unstructured.Unstructured{customObject("Widget", "watched-widget", ""), customObject("Gizmo", "resynced-gizmo", "")} {
			obj.SetNamespace("payments")
			_ = k8sClient.Delete(ctx, &obj)
		}
		deleteObject(ctx, &corev1alpha1.Repository{ObjectMeta: metav1.ObjectMeta{Name: "platform", Namespace: "default"}})
		deleteObject(ctx, &rbacv1.ClusterRoleBinding{ObjectMeta: metav1.ObjectMeta{Name: controllerAccount}})
		deleteObject(ctx, &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: controllerAccount}})
		deleteObject(ctx, &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: controllerAccount, Namespace: "default"}})
	})

	It("records managed kinds and repairs a watched kind through its watch", func() {
		settleApplication(watchedApp)
		app := &corev1alpha1.Application{}
		Expect(k8sClient.Get(ctx, watchedApp, app)).To(Succeed())
		Expect(app.Status.ManagedKinds).To(Equal([]corev1alpha1.ManagedKind{{APIVersion: "example.com/v1", Kind: "Widget"}}))

		recreatedWithin(customObject("Widget", "watched-widget", ""), 5*time.Second)
	})

	It("repairs a kind the controller may not watch on the resync interval", func() {
		settleApplication(resyncedApp)
		recreatedWithin(customObject("Gizmo", "resynced-gizmo", ""), 3*resync)
	})
})

// settleApplication waits until the Application is Healthy and quiet, so only
// a watch event or the resync can trigger its next reconcile.
func settleApplication(key types.NamespacedName) {
	settled := &corev1alpha1.Application{}
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, key, settled)).To(Succeed())
		g.Expect(settled.Status.Health.State).To(Equal(corev1alpha1.HealthStateHealthy))
	}, 30*time.Second, 200*time.Millisecond).Should(Succeed())
	Consistently(func(g Gomega) {
		current := &corev1alpha1.Application{}
		g.Expect(k8sClient.Get(ctx, key, current)).To(Succeed())
		g.Expect(current.ResourceVersion).To(Equal(settled.ResourceVersion))
	}, 2*time.Second, 200*time.Millisecond).Should(Succeed())
}

// recreatedWithin deletes a managed object in payments and expects Kuvryn Sync to
// recreate it within the given time.
func recreatedWithin(obj unstructured.Unstructured, within time.Duration) {
	obj.SetNamespace("payments")
	key := client.ObjectKeyFromObject(&obj)
	live := obj.DeepCopy()
	Expect(k8sClient.Get(ctx, key, live)).To(Succeed())
	deleted := live.GetUID()
	Expect(k8sClient.Delete(ctx, live)).To(Succeed())
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, key, live)).To(Succeed())
		g.Expect(live.GetUID()).NotTo(Equal(deleted))
	}, within, 200*time.Millisecond).Should(Succeed())
}
