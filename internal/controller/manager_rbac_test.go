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
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"

	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/rest"
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

// The manager runs with a real token for a service account bound only to the
// generated config/rbac/role.yaml, proving the least-privilege role is enough
// to watch, impersonate, apply, and self-heal.
var _ = Describe("Manager with the generated controller role", Ordered, func() {
	const (
		controllerAccount = "kuvryn-sync-test-controller"
		appName           = "least-privilege-app"
		configName        = "least-privilege-config"
	)
	var stop context.CancelFunc

	BeforeAll(func() {
		ensureNamespace(ctx, "payments")
		role := generatedManagerRole()
		role.Name = controllerAccount
		Expect(k8sClient.Create(ctx, role)).To(Succeed())
		Expect(k8sClient.Create(ctx, &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: controllerAccount, Namespace: "default"}})).To(Succeed())
		Expect(k8sClient.Create(ctx, &rbacv1.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: controllerAccount},
			RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: controllerAccount},
			Subjects:   []rbacv1.Subject{{Kind: rbacv1.ServiceAccountKind, Name: controllerAccount, Namespace: "default"}},
		})).To(Succeed())

		controllerConfig := serviceAccountConfig(controllerAccount)
		mgr, err := ctrl.NewManager(controllerConfig, ctrl.Options{
			Scheme:                 k8sClient.Scheme(),
			Metrics:                metricsserver.Options{BindAddress: "0"},
			HealthProbeBindAddress: "0",
			Controller:             config.Controller{SkipNameValidation: ptr.To(true)},
			Client:                 client.Options{Cache: &client.CacheOptions{DisableFor: []client.Object{&corev1.Secret{}}}},
		})
		Expect(err).NotTo(HaveOccurred())
		capture := &capturingRenderer{objects: []unstructured.Unstructured{namedConfigMap(configName, "desired")}}
		Expect((&ApplicationReconciler{
			Client:         mgr.GetClient(),
			Scheme:         mgr.GetScheme(),
			SourceResolver: fakeResolver{},
			Renderers: func(corev1alpha1.RenderType) (renderer.Renderer, error) {
				return capture, nil
			},
			Impersonation:         impersonate.New(controllerConfig, client.Options{Scheme: mgr.GetScheme()}),
			DefaultServiceAccount: testServiceAccount,
		}).SetupWithManager(mgr)).To(Succeed())

		var managerCtx context.Context
		managerCtx, stop = context.WithCancel(ctx)
		go func() {
			defer GinkgoRecover()
			Expect(mgr.Start(managerCtx)).To(Succeed())
		}()
	})

	AfterAll(func() {
		stop()
		deleteObject(ctx, &corev1alpha1.Application{ObjectMeta: metav1.ObjectMeta{Name: appName, Namespace: "default"}})
		deleteObject(ctx, &corev1alpha1.Repository{ObjectMeta: metav1.ObjectMeta{Name: "platform", Namespace: "default"}})
		deleteObject(ctx, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "platform-git", Namespace: "default"}})
		deleteObject(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: configName, Namespace: "payments"}})
		deleteObject(ctx, &rbacv1.ClusterRoleBinding{ObjectMeta: metav1.ObjectMeta{Name: controllerAccount}})
		deleteObject(ctx, &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: controllerAccount}})
		deleteObject(ctx, &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: controllerAccount, Namespace: "default"}})
		deleteApplicationRevisions(ctx, appName)
	})

	It("syncs an Application and recreates a deleted managed object", func() {
		Expect(k8sClient.Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "platform-git", Namespace: "default", Labels: map[string]string{GitCredentialsLabel: "true"}},
			Data:       map[string][]byte{"token": []byte("git-token")},
		})).To(Succeed())
		Expect(k8sClient.Create(ctx, &corev1alpha1.Repository{
			ObjectMeta: metav1.ObjectMeta{Name: "platform", Namespace: "default"},
			Spec: corev1alpha1.RepositorySpec{
				Type: corev1alpha1.RepositoryTypeGit,
				Git: &corev1alpha1.GitRepositorySpec{
					URL:      "https://example.com/platform.git",
					Revision: "main",
					Auth:     &corev1alpha1.GitAuthSpec{SecretRef: &corev1alpha1.SecretReference{Name: "platform-git"}},
				},
			},
		})).To(Succeed())
		app := newApplication(appName, corev1alpha1.RenderTypeYAML)
		app.Spec.Sync.Automatic = true
		Expect(k8sClient.Create(ctx, app)).To(Succeed())

		live := &corev1.ConfigMap{}
		liveKey := client.ObjectKey{Name: configName, Namespace: "payments"}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, liveKey, live)).To(Succeed())
			g.Expect(live.Data).To(HaveKeyWithValue("key", "desired"))
		}, 30*time.Second, 200*time.Millisecond).Should(Succeed())

		// Let the initial sync settle so only a watch event can trigger the
		// next reconcile.
		appKey := client.ObjectKeyFromObject(app)
		settled := &corev1alpha1.Application{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, appKey, settled)).To(Succeed())
			g.Expect(settled.Status.Health.State).To(Equal(corev1alpha1.HealthStateHealthy))
		}, 30*time.Second, 200*time.Millisecond).Should(Succeed())
		Consistently(func(g Gomega) {
			current := &corev1alpha1.Application{}
			g.Expect(k8sClient.Get(ctx, appKey, current)).To(Succeed())
			g.Expect(current.ResourceVersion).To(Equal(settled.ResourceVersion))
		}, 2*time.Second, 200*time.Millisecond).Should(Succeed())

		// Deleting a managed object is drift no other field manager owns, so
		// only the metadata watch can bring Kuvryn Sync back to recreate it.
		deleted := live.UID
		Expect(k8sClient.Delete(ctx, live)).To(Succeed())
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, liveKey, live)).To(Succeed())
			g.Expect(live.UID).NotTo(Equal(deleted))
			g.Expect(live.Data).To(HaveKeyWithValue("key", "desired"))
		}, 30*time.Second, 200*time.Millisecond).Should(Succeed())
	})
})

func generatedManagerRole() *rbacv1.ClusterRole {
	file, err := os.Open(filepath.Join("..", "..", "config", "rbac", "role.yaml"))
	Expect(err).NotTo(HaveOccurred())
	defer func() { _ = file.Close() }()
	decoder := utilyaml.NewYAMLOrJSONDecoder(file, 4096)
	for {
		role := &rbacv1.ClusterRole{}
		err := decoder.Decode(role)
		if errors.Is(err, io.EOF) {
			break
		}
		Expect(err).NotTo(HaveOccurred())
		if role.Name == "manager-role" {
			return role
		}
	}
	Fail("config/rbac/role.yaml has no manager-role ClusterRole")
	return nil
}

// serviceAccountConfig authenticates as a service account with a real token
// rather than the envtest admin certificate.
func serviceAccountConfig(name string) *rest.Config {
	request := &authenticationv1.TokenRequest{Spec: authenticationv1.TokenRequestSpec{ExpirationSeconds: ptr.To(int64(3600))}}
	account := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"}}
	Expect(k8sClient.SubResource("token").Create(ctx, account, request)).To(Succeed())
	accountConfig := rest.AnonymousClientConfig(cfg)
	accountConfig.BearerToken = request.Status.Token
	return accountConfig
}

func namedConfigMap(name, value string) unstructured.Unstructured {
	obj := configMapObject("", value)
	obj.SetName(name)
	return obj
}
