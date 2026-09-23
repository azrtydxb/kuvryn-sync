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

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1alpha1 "github.com/azrtydxb/solder/api/v1alpha1"
)

var _ = Describe("Application service account impersonation", func() {
	const (
		appName    = "tenant-app"
		tenant     = "payments-deployer"
		escalation = "tenant-escalation"
	)
	ctx := context.Background()
	key := types.NamespacedName{Name: appName, Namespace: "default"}

	BeforeEach(func() {
		ensureNamespace(ctx, "payments")
	})

	AfterEach(func() {
		deleteObject(ctx, &corev1alpha1.Application{ObjectMeta: metav1.ObjectMeta{Name: appName, Namespace: "default"}})
		deleteObject(ctx, &corev1alpha1.Repository{ObjectMeta: metav1.ObjectMeta{Name: "platform", Namespace: "default"}})
		deleteObject(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "app-config", Namespace: "payments"}})
		deleteObject(ctx, &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: tenant, Namespace: "default"}})
		deleteObject(ctx, &rbacv1.Role{ObjectMeta: metav1.ObjectMeta{Name: tenant, Namespace: "payments"}})
		deleteObject(ctx, &rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{Name: tenant, Namespace: "payments"}})
		deleteObject(ctx, &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: tenant}})
		deleteObject(ctx, &rbacv1.ClusterRoleBinding{ObjectMeta: metav1.ObjectMeta{Name: tenant}})
		deleteObject(ctx, &rbacv1.ClusterRoleBinding{ObjectMeta: metav1.ObjectMeta{Name: escalation}})
		deleteApplicationRevisions(ctx, appName)
	})

	It("starts a new Revision when the service account changes", func() {
		app := newApplication(appName, corev1alpha1.RenderTypeYAML)
		before, _, err := revisionIdentity(app, "resolved-sha", "restricted")
		Expect(err).NotTo(HaveOccurred())
		after, _, err := revisionIdentity(app, "resolved-sha", "deployer")
		Expect(err).NotTo(HaveOccurred())
		Expect(after).NotTo(Equal(before))
	})

	It("refuses an Application when no service account is configured", func() {
		createRepository(ctx)
		app := newApplication(appName, corev1alpha1.RenderTypeYAML)
		Expect(k8sClient.Create(ctx, app)).To(Succeed())
		app.Status.ServiceAccountName = "previous-deployer"
		Expect(k8sClient.Status().Update(ctx, app)).To(Succeed())

		reconciler := newApplicationReconciler([]unstructured.Unstructured{configMapObject("", "desired")}, nil)
		reconciler.DefaultServiceAccount = ""
		_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: key})
		Expect(err).NotTo(HaveOccurred())

		updated := &corev1alpha1.Application{}
		Expect(k8sClient.Get(ctx, key, updated)).To(Succeed())
		ready := apimeta.FindStatusCondition(updated.Status.Conditions, "Ready")
		Expect(ready).NotTo(BeNil())
		Expect(ready.Reason).To(Equal("ServiceAccountRequired"))
		Expect(updated.Status.State).To(Equal(corev1alpha1.HealthStateDegraded))
		Expect(updated.Status.ServiceAccountName).To(BeEmpty())
		Expect(listApplicationRevisions(ctx, appName).Items).To(BeEmpty())
	})

	It("plans with the tenant account but cannot apply what its RBAC forbids", func() {
		createRepository(ctx)
		createTenant(ctx, tenant, allConfigMapVerbs, true)
		app := newApplication(appName, corev1alpha1.RenderTypeYAML)
		app.Spec.ServiceAccountName = tenant
		app.Spec.Sync.Automatic = true
		Expect(k8sClient.Create(ctx, app)).To(Succeed())

		reconciler := newApplicationReconciler([]unstructured.Unstructured{configMapObject("", "desired"), clusterAdminBinding(escalation, tenant)}, nil)
		recorder := record.NewFakeRecorder(20)
		reconciler.Recorder = recorder
		_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: key})
		Expect(err).NotTo(HaveOccurred())

		updated := &corev1alpha1.Application{}
		Expect(k8sClient.Get(ctx, key, updated)).To(Succeed())
		Expect(updated.Status.ServiceAccountName).To(Equal(tenant))

		revisions := listApplicationRevisions(ctx, appName)
		Expect(revisions.Items).To(HaveLen(1))
		revision := revisions.Items[0]
		Expect(revision.Status.Plan.Summary.Create).To(Equal(int32(2)))
		Expect(revision.Status.Phase).To(Equal(corev1alpha1.RevisionPhaseFailed))
		Expect(revision.Status.Failure).NotTo(BeNil())
		Expect(revision.Status.Failure.Reason).To(Equal("Forbidden"))
		Expect(revision.Status.Failure.Retryable).To(BeFalse())
		Expect(revision.Status.Failure.Message).To(ContainSubstring("clusterrolebindings"))

		err = k8sClient.Get(ctx, client.ObjectKey{Name: escalation}, &rbacv1.ClusterRoleBinding{})
		Expect(apierrors.IsNotFound(err)).To(BeTrue(), "tenant escalated through Solder")
		Expect(drainEvents(recorder)).To(ContainElement(And(ContainSubstring("PruneInventoryIncomplete"), ContainSubstring("Secret"))))

		By("keeping the Forbidden failure once retries are exhausted")
		_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: key})
		Expect(err).NotTo(HaveOccurred())
		revision = listApplicationRevisions(ctx, appName).Items[0]
		Expect(revision.Status.Failure.Reason).To(Equal("Forbidden"))
		Expect(k8sClient.Get(ctx, key, updated)).To(Succeed())
		ready := apimeta.FindStatusCondition(updated.Status.Conditions, "Ready")
		Expect(ready).NotTo(BeNil())
		Expect(ready.Reason).To(Equal("RetryBlocked"))
		Expect(ready.Message).To(ContainSubstring("Forbidden"))
	})

	It("fails planning as Forbidden when the account may not read desired objects", func() {
		createRepository(ctx)
		createTenant(ctx, tenant, []string{"create", "patch"}, false)
		app := newApplication(appName, corev1alpha1.RenderTypeYAML)
		app.Spec.ServiceAccountName = tenant
		app.Spec.Sync.Prune = false
		Expect(k8sClient.Create(ctx, app)).To(Succeed())

		reconciler := newApplicationReconciler([]unstructured.Unstructured{configMapObject("", "desired")}, nil)
		_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: key})
		Expect(err).NotTo(HaveOccurred())

		revision := listApplicationRevisions(ctx, appName).Items[0]
		Expect(revision.Status.Failure).NotTo(BeNil())
		Expect(revision.Status.Failure.Reason).To(Equal("Forbidden"))
		Expect(revision.Status.Failure.Message).To(ContainSubstring("get"))
	})

	It("prunes as the account and fails as Forbidden when it may not delete", func() {
		createRepository(ctx)
		createTenant(ctx, tenant, []string{"get", "list", "watch", "create", "patch"}, false)
		stale := configMapObject("payments", "stale")
		stale.SetName("stale-config")
		stale.SetLabels(map[string]string{"solder.io/application": appName, "solder.io/application-namespace": "default"})
		Expect(k8sClient.Create(ctx, &stale)).To(Succeed())
		DeferCleanup(deleteObject, ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "stale-config", Namespace: "payments"}})
		app := newApplication(appName, corev1alpha1.RenderTypeYAML)
		app.Spec.ServiceAccountName = tenant
		app.Spec.Sync.Automatic = true
		Expect(k8sClient.Create(ctx, app)).To(Succeed())

		reconciler := newApplicationReconciler([]unstructured.Unstructured{configMapObject("", "desired")}, nil)
		_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: key})
		Expect(err).NotTo(HaveOccurred())

		revision := listApplicationRevisions(ctx, appName).Items[0]
		Expect(revision.Status.Failure).NotTo(BeNil())
		Expect(revision.Status.Failure.Reason).To(Equal("Forbidden"))
		Expect(revision.Status.Failure.Message).To(ContainSubstring("delete"))
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "stale-config", Namespace: "payments"}, &corev1.ConfigMap{})).To(Succeed())
	})

	It("deletes managed resources as the account and reports when it may not", func() {
		createRepository(ctx)
		createTenant(ctx, tenant, []string{"get", "list", "watch"}, false)
		app := newApplication(appName, corev1alpha1.RenderTypeYAML)
		app.Spec.ServiceAccountName = tenant
		app.Spec.DeletionPolicy = corev1alpha1.DeletionPolicyDeleteManagedResources
		controllerutil.AddFinalizer(app, applicationFinalizer)
		Expect(k8sClient.Create(ctx, app)).To(Succeed())
		managed := configMapObject("payments", "managed")
		managed.SetLabels(map[string]string{"solder.io/application": appName, "solder.io/application-namespace": "default"})
		Expect(k8sClient.Create(ctx, &managed)).To(Succeed())
		Expect(k8sClient.Delete(ctx, app)).To(Succeed())

		reconciler := newApplicationReconciler(nil, nil)
		recorder := record.NewFakeRecorder(20)
		reconciler.Recorder = recorder
		_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: key})
		Expect(apierrors.IsForbidden(err)).To(BeTrue())

		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "app-config", Namespace: "payments"}, &corev1.ConfigMap{})).To(Succeed())
		Expect(k8sClient.Get(ctx, key, &corev1alpha1.Application{})).To(Succeed())
		Expect(drainEvents(recorder)).To(ContainElement(ContainSubstring("Forbidden")))
	})

	It("orphans managed resources on delete when no service account is configured", func() {
		createRepository(ctx)
		app := newApplication(appName, corev1alpha1.RenderTypeYAML)
		app.Spec.DeletionPolicy = corev1alpha1.DeletionPolicyDeleteManagedResources
		controllerutil.AddFinalizer(app, applicationFinalizer)
		Expect(k8sClient.Create(ctx, app)).To(Succeed())
		managed := configMapObject("payments", "kept")
		managed.SetLabels(map[string]string{"solder.io/application": appName, "solder.io/application-namespace": "default"})
		Expect(k8sClient.Create(ctx, &managed)).To(Succeed())
		Expect(k8sClient.Delete(ctx, app)).To(Succeed())

		reconciler := newApplicationReconciler(nil, nil)
		reconciler.DefaultServiceAccount = ""
		_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: key})
		Expect(err).NotTo(HaveOccurred())

		err = k8sClient.Get(ctx, key, &corev1alpha1.Application{})
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "app-config", Namespace: "payments"}, &corev1.ConfigMap{})).To(Succeed())
	})
})

var allConfigMapVerbs = []string{"get", "list", "watch", "create", "update", "patch", "delete"}

// createTenant creates a service account with the given ConfigMap verbs in the
// payments namespace and, optionally, read access to ClusterRoleBindings.
func createTenant(ctx context.Context, name string, configMapVerbs []string, readClusterRoleBindings bool) {
	subject := rbacv1.Subject{Kind: rbacv1.ServiceAccountKind, Name: name, Namespace: "default"}
	Expect(k8sClient.Create(ctx, &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"}})).To(Succeed())
	Expect(k8sClient.Create(ctx, &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "payments"},
		Rules:      []rbacv1.PolicyRule{{APIGroups: []string{""}, Resources: []string{"configmaps"}, Verbs: configMapVerbs}},
	})).To(Succeed())
	Expect(k8sClient.Create(ctx, &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "payments"},
		RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "Role", Name: name},
		Subjects:   []rbacv1.Subject{subject},
	})).To(Succeed())
	if !readClusterRoleBindings {
		return
	}
	Expect(k8sClient.Create(ctx, &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Rules:      []rbacv1.PolicyRule{{APIGroups: []string{rbacv1.GroupName}, Resources: []string{"clusterrolebindings"}, Verbs: []string{"get", "list", "watch"}}},
	})).To(Succeed())
	Expect(k8sClient.Create(ctx, &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: name},
		Subjects:   []rbacv1.Subject{subject},
	})).To(Succeed())
}

func drainEvents(recorder *record.FakeRecorder) []string {
	events := []string{}
	for {
		select {
		case event := <-recorder.Events:
			events = append(events, event)
		default:
			return events
		}
	}
}

// clusterAdminBinding is desired state that would grant the tenant cluster-admin.
func clusterAdminBinding(name, serviceAccount string) unstructured.Unstructured {
	return unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "rbac.authorization.k8s.io/v1",
		"kind":       "ClusterRoleBinding",
		"metadata":   map[string]any{"name": name},
		"roleRef":    map[string]any{"apiGroup": rbacv1.GroupName, "kind": "ClusterRole", "name": "cluster-admin"},
		"subjects":   []any{map[string]any{"kind": "ServiceAccount", "name": serviceAccount, "namespace": "default"}},
	}}
}
