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
	"encoding/json"
	"errors"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1alpha1 "github.com/azrtydxb/solder/api/v1alpha1"
	"github.com/azrtydxb/solder/internal/impersonate"
	"github.com/azrtydxb/solder/internal/renderer"
	"github.com/azrtydxb/solder/internal/source"
)

var _ = Describe("Application Controller", func() {
	const resourceName = "test-application"

	ctx := context.Background()
	typeNamespacedName := types.NamespacedName{Name: resourceName, Namespace: "default"}

	BeforeEach(func() {
		ensureNamespace(ctx, "payments")
	})

	AfterEach(func() {
		deleteObject(ctx, &corev1alpha1.Application{ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: "default"}})
		deleteObject(ctx, &corev1alpha1.Repository{ObjectMeta: metav1.ObjectMeta{Name: "platform", Namespace: "default"}})
		deleteObject(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "app-config", Namespace: "payments"}})
		deleteObject(ctx, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "app-secret", Namespace: "payments"}})
		deleteApplicationRevisions(ctx, resourceName)
	})

	It("plans rendered desired state into a deterministic Revision", func() {
		createRepository(ctx)
		resource := newApplication(resourceName, corev1alpha1.RenderTypeYAML)
		Expect(k8sClient.Create(ctx, resource)).To(Succeed())

		controllerReconciler := newApplicationReconciler([]unstructured.Unstructured{configMapObject("", "desired")}, nil)
		_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
		Expect(err).NotTo(HaveOccurred())

		updated := &corev1alpha1.Application{}
		Expect(k8sClient.Get(ctx, typeNamespacedName, updated)).To(Succeed())
		Expect(updated.Status.DesiredRevision).To(Equal("resolved-sha"))
		Expect(updated.Status.Health.State).To(Equal(corev1alpha1.HealthStateUnknown))
		Expect(updated.Status.Sync.State).To(Equal(corev1alpha1.SyncStateAwaitingApproval))

		revisions := listApplicationRevisions(ctx, resourceName)
		Expect(revisions.Items).To(HaveLen(1))
		revision := revisions.Items[0]
		Expect(revision.Spec.Source.Revision).To(Equal("resolved-sha"))
		Expect(revision.Status.Phase).To(Equal(corev1alpha1.RevisionPhaseAwaitingApproval))
		Expect(revision.Status.Plan.Summary.Create).To(Equal(int32(1)))
		Expect(revision.Status.Plan.Resources).To(HaveLen(1))
		Expect(revision.Status.Plan.Resources[0].Resource.Namespace).To(Equal("payments"))
		Expect(revision.Status.Plan.Resources[0].Action).To(Equal(corev1alpha1.PlanActionCreate))
	})

	It("adds a finalizer when deletion should remove managed resources", func() {
		createRepository(ctx)
		resource := newApplication(resourceName, corev1alpha1.RenderTypeYAML)
		resource.Spec.DeletionPolicy = corev1alpha1.DeletionPolicyDeleteManagedResources
		Expect(k8sClient.Create(ctx, resource)).To(Succeed())

		controllerReconciler := newApplicationReconciler([]unstructured.Unstructured{configMapObject("", "desired")}, nil)
		_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
		Expect(err).NotTo(HaveOccurred())

		updated := &corev1alpha1.Application{}
		Expect(k8sClient.Get(ctx, typeNamespacedName, updated)).To(Succeed())
		Expect(updated.Finalizers).To(ContainElement("applications.solder.io/finalizer"))
	})

	It("prunes stale managed resources during automatic sync", func() {
		createRepository(ctx)
		stale := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "app-config", Namespace: "payments", Labels: map[string]string{"solder.io/application": resourceName}},
			Data:       map[string]string{"key": "stale"},
		}
		Expect(k8sClient.Create(ctx, stale)).To(Succeed())
		resource := newApplication(resourceName, corev1alpha1.RenderTypeYAML)
		resource.Spec.Sync.Automatic = true
		resource.Spec.Sync.Prune = true
		Expect(k8sClient.Create(ctx, resource)).To(Succeed())

		controllerReconciler := newApplicationReconciler([]unstructured.Unstructured{}, nil)
		_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
		Expect(err).NotTo(HaveOccurred())

		err = k8sClient.Get(ctx, types.NamespacedName{Name: "app-config", Namespace: "payments"}, &corev1.ConfigMap{})
		Expect(client.IgnoreNotFound(err)).To(Succeed())
		Expect(err).To(HaveOccurred())
		revision := listApplicationRevisions(ctx, resourceName).Items[0]
		Expect(revision.Status.Plan.Summary.Delete).To(Equal(int32(1)))
	})

	It("rejects prune when a managed resource opts out", func() {
		createRepository(ctx)
		stale := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "app-config",
				Namespace:   "payments",
				Labels:      map[string]string{"solder.io/application": resourceName},
				Annotations: map[string]string{"solder.io/prune": "disabled"},
			},
			Data: map[string]string{"key": "stale"},
		}
		Expect(k8sClient.Create(ctx, stale)).To(Succeed())
		resource := newApplication(resourceName, corev1alpha1.RenderTypeYAML)
		resource.Spec.Sync.Automatic = true
		resource.Spec.Sync.Prune = true
		Expect(k8sClient.Create(ctx, resource)).To(Succeed())

		controllerReconciler := newApplicationReconciler([]unstructured.Unstructured{}, nil)
		_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
		Expect(err).NotTo(HaveOccurred())

		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "app-config", Namespace: "payments"}, &corev1.ConfigMap{})).To(Succeed())
		revision := listApplicationRevisions(ctx, resourceName).Items[0]
		Expect(revision.Status.Phase).To(Equal(corev1alpha1.RevisionPhaseFailed))
		Expect(revision.Status.Failure).NotTo(BeNil())
		Expect(revision.Status.Failure.Reason).To(Equal("PruneFailure"))
	})

	It("applies automatic syncs and records health", func() {
		createRepository(ctx)
		resource := newApplication(resourceName, corev1alpha1.RenderTypeYAML)
		resource.Spec.Sync.Automatic = true
		Expect(k8sClient.Create(ctx, resource)).To(Succeed())

		controllerReconciler := newApplicationReconciler([]unstructured.Unstructured{configMapObject("", "desired")}, nil)
		_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
		Expect(err).NotTo(HaveOccurred())

		live := &corev1.ConfigMap{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "app-config", Namespace: "payments"}, live)).To(Succeed())
		Expect(live.Data).To(HaveKeyWithValue("key", "desired"))
		Expect(live.Labels).To(HaveKeyWithValue("solder.io/application", resourceName))
		Expect(live.Labels).To(HaveKeyWithValue("solder.io/application-namespace", "default"))

		updated := &corev1alpha1.Application{}
		Expect(k8sClient.Get(ctx, typeNamespacedName, updated)).To(Succeed())
		Expect(updated.Status.Sync.State).To(Equal(corev1alpha1.SyncStateSynced))
		Expect(updated.Status.Health.State).To(Equal(corev1alpha1.HealthStateHealthy))
		Expect(updated.Status.DeployedRevision).To(Equal("resolved-sha"))

		revision := listApplicationRevisions(ctx, resourceName).Items[0]
		Expect(revision.Status.Phase).To(Equal(corev1alpha1.RevisionPhaseHealthy))
		Expect(revision.Status.Health.Healthy).To(Equal(int32(1)))
	})

	It("applies only the exact approved manual Revision", func() {
		createRepository(ctx)
		resource := newApplication(resourceName, corev1alpha1.RenderTypeYAML)
		resource.SetAnnotations(map[string]string{"solder.io/approved-revision": "wrong-revision"})
		Expect(k8sClient.Create(ctx, resource)).To(Succeed())

		controllerReconciler := newApplicationReconciler([]unstructured.Unstructured{configMapObject("", "desired")}, nil)
		_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
		Expect(err).NotTo(HaveOccurred())
		revision := listApplicationRevisions(ctx, resourceName).Items[0]
		Expect(revision.Status.Phase).To(Equal(corev1alpha1.RevisionPhaseAwaitingApproval))

		approve(ctx, typeNamespacedName, revision, "alice@example.com")
		_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
		Expect(err).NotTo(HaveOccurred())

		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "app-config", Namespace: "payments"}, &corev1.ConfigMap{})).To(Succeed())
		applied := listApplicationRevisions(ctx, resourceName).Items[0]
		Expect(applied.Status.Phase).To(Equal(corev1alpha1.RevisionPhaseHealthy))
		Expect(applied.Status.Approval).NotTo(BeNil())
		Expect(applied.Status.Approval.ApprovedBy).To(Equal("alice@example.com"))
		Expect(applied.Status.Approval.PlanDigest).To(Equal(revision.Status.Plan.Digest))
	})

	It("ignores an approval without a recorded approver and digest", func() {
		createRepository(ctx)
		Expect(k8sClient.Create(ctx, newApplication(resourceName, corev1alpha1.RenderTypeYAML))).To(Succeed())
		controllerReconciler := newApplicationReconciler([]unstructured.Unstructured{configMapObject("", "desired")}, nil)
		_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
		Expect(err).NotTo(HaveOccurred())
		revision := listApplicationRevisions(ctx, resourceName).Items[0]

		updated := &corev1alpha1.Application{}
		Expect(k8sClient.Get(ctx, typeNamespacedName, updated)).To(Succeed())
		updated.SetAnnotations(map[string]string{corev1alpha1.ApprovedRevisionAnnotation: revision.Name})
		Expect(k8sClient.Update(ctx, updated)).To(Succeed())
		_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
		Expect(err).NotTo(HaveOccurred())

		Expect(listApplicationRevisions(ctx, resourceName).Items[0].Status.Phase).To(Equal(corev1alpha1.RevisionPhaseAwaitingApproval))
		Expect(apierrors.IsNotFound(k8sClient.Get(ctx, types.NamespacedName{Name: "app-config", Namespace: "payments"}, &corev1.ConfigMap{}))).To(BeTrue())
	})

	It("requires a new approval when the plan changes after approval", func() {
		createRepository(ctx)
		Expect(k8sClient.Create(ctx, newApplication(resourceName, corev1alpha1.RenderTypeYAML))).To(Succeed())
		capture := &capturingRenderer{objects: []unstructured.Unstructured{configMapObject("", "reviewed")}}
		controllerReconciler := newApplicationReconciler(nil, capture)
		recorder := record.NewFakeRecorder(20)
		controllerReconciler.Recorder = recorder
		_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
		Expect(err).NotTo(HaveOccurred())
		approve(ctx, typeNamespacedName, listApplicationRevisions(ctx, resourceName).Items[0], "alice@example.com")

		capture.objects = []unstructured.Unstructured{configMapObject("", "changed-after-review")}
		_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
		Expect(err).NotTo(HaveOccurred())

		revision := listApplicationRevisions(ctx, resourceName).Items[0]
		Expect(revision.Status.Phase).To(Equal(corev1alpha1.RevisionPhaseAwaitingApproval))
		Expect(revision.Status.Approval).To(BeNil())
		Expect(apierrors.IsNotFound(k8sClient.Get(ctx, types.NamespacedName{Name: "app-config", Namespace: "payments"}, &corev1.ConfigMap{}))).To(BeTrue())
		Expect(drainEvents(recorder)).To(ContainElement(ContainSubstring("ApprovalStale")))
	})

	It("uses live state when building the plan", func() {
		createRepository(ctx)
		Expect(k8sClient.Create(ctx, &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "app-config", Namespace: "payments"},
			Data:       map[string]string{"key": "live"},
		})).To(Succeed())
		resource := newApplication(resourceName, corev1alpha1.RenderTypeYAML)
		Expect(k8sClient.Create(ctx, resource)).To(Succeed())

		controllerReconciler := newApplicationReconciler([]unstructured.Unstructured{configMapObject("", "desired")}, nil)
		_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
		Expect(err).NotTo(HaveOccurred())

		revision := listApplicationRevisions(ctx, resourceName).Items[0]
		Expect(revision.Status.Plan.Summary.Update).To(Equal(int32(1)))
		Expect(revision.Status.Plan.Resources[0].Action).To(Equal(corev1alpha1.PlanActionUpdate))
	})

	It("fails validation before reading live state", func() {
		createRepository(ctx)
		resource := newApplication(resourceName, corev1alpha1.RenderTypeYAML)
		Expect(k8sClient.Create(ctx, resource)).To(Succeed())

		invalid := unstructured.Unstructured{Object: map[string]any{"apiVersion": "v1", "kind": "ConfigMap"}}
		controllerReconciler := newApplicationReconciler([]unstructured.Unstructured{invalid}, nil)
		_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
		Expect(err).NotTo(HaveOccurred())

		revision := listApplicationRevisions(ctx, resourceName).Items[0]
		Expect(revision.Status.Phase).To(Equal(corev1alpha1.RevisionPhaseFailed))
		Expect(revision.Status.Failure).NotTo(BeNil())
		Expect(revision.Status.Failure.Reason).To(Equal("ValidationFailure"))
		updated := &corev1alpha1.Application{}
		Expect(k8sClient.Get(ctx, typeNamespacedName, updated)).To(Succeed())
		Expect(updated.Status.Sync.State).To(Equal(corev1alpha1.SyncStateOutOfSync))
		Expect(updated.Status.Health.State).To(Equal(corev1alpha1.HealthStateDegraded))
	})

	It("passes Helm render options from the Application spec", func() {
		createRepository(ctx)
		resource := newApplication(resourceName, corev1alpha1.RenderTypeHelm)
		resource.Spec.Source.Render.Helm = &corev1alpha1.HelmRenderSpec{ReleaseName: "payments", ValuesFiles: []string{"values/prod.yaml"}}
		Expect(k8sClient.Create(ctx, resource)).To(Succeed())
		capture := &capturingRenderer{objects: []unstructured.Unstructured{configMapObject("", "desired")}}

		controllerReconciler := newApplicationReconciler(nil, capture)
		_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
		Expect(err).NotTo(HaveOccurred())

		Expect(capture.input.Workspace).To(Equal("/tmp/solder-workspace"))
		Expect(capture.input.Path).To(Equal("apps/payments"))
		Expect(capture.input.ReleaseName).To(Equal("payments"))
		Expect(capture.input.ValuesFiles).To(Equal([]string{"values/prod.yaml"}))
	})

	It("records render failures on the Revision without leaking secret data", func() {
		createRepository(ctx)
		resource := newApplication(resourceName, corev1alpha1.RenderTypeYAML)
		Expect(k8sClient.Create(ctx, resource)).To(Succeed())

		controllerReconciler := newApplicationReconciler(nil, &capturingRenderer{err: errors.New("render failed: token REDACTED")})
		_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
		Expect(err).NotTo(HaveOccurred())

		revision := listApplicationRevisions(ctx, resourceName).Items[0]
		Expect(revision.Status.Phase).To(Equal(corev1alpha1.RevisionPhaseFailed))
		Expect(revision.Status.Failure).NotTo(BeNil())
		Expect(revision.Status.Failure.Reason).To(Equal("RenderFailure"))
		Expect(revision.Status.Failure.Message).NotTo(ContainSubstring("password"))
	})

	It("bounds and redacts Secret plan details", func() {
		createRepository(ctx)
		resource := newApplication(resourceName, corev1alpha1.RenderTypeYAML)
		Expect(k8sClient.Create(ctx, resource)).To(Succeed())

		controllerReconciler := newApplicationReconciler([]unstructured.Unstructured{secretObject("app-secret")}, nil)
		controllerReconciler.PlanLimit = 1
		_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
		Expect(err).NotTo(HaveOccurred())

		revision := listApplicationRevisions(ctx, resourceName).Items[0]
		Expect(revision.Status.Plan.Resources).To(HaveLen(1))
		Expect(revision.Status.Plan.Resources[0].Changes).NotTo(BeEmpty())
		Expect(revision.Status.Plan.Resources[0].Changes[0].Redacted).To(BeTrue())
		Expect(revision.Status.Plan.Resources[0].Changes[0].After).To(ContainSubstring("REDACTED"))
		encoded, err := jsonMarshal(revision.Status.Plan)
		Expect(err).NotTo(HaveOccurred())
		Expect(encoded).NotTo(ContainSubstring("super-secret"))
	})
})

type fakeResolver struct{}

func (fakeResolver) Resolve(ctx context.Context, repository source.GitRepository) (source.ResolvedSource, error) {
	return source.ResolvedSource{Revision: "resolved-sha", CacheDir: "/tmp/solder-workspace"}, nil
}

type capturingRenderer struct {
	input   renderer.Input
	objects []unstructured.Unstructured
	err     error
}

func (r *capturingRenderer) Render(ctx context.Context, input renderer.Input) ([]unstructured.Unstructured, error) {
	r.input = input
	return r.objects, r.err
}

func newApplicationReconciler(objects []unstructured.Unstructured, capture *capturingRenderer) *ApplicationReconciler {
	if capture == nil {
		capture = &capturingRenderer{objects: objects}
	}
	return &ApplicationReconciler{
		Client:         k8sClient,
		Scheme:         k8sClient.Scheme(),
		SourceResolver: fakeResolver{},
		Renderers: func(renderType corev1alpha1.RenderType) (renderer.Renderer, error) {
			return capture, nil
		},
		Impersonation:         impersonate.New(cfg, client.Options{Scheme: k8sClient.Scheme()}),
		DefaultServiceAccount: testServiceAccount,
	}
}

func newApplication(name string, renderType corev1alpha1.RenderType) *corev1alpha1.Application {
	return &corev1alpha1.Application{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: corev1alpha1.ApplicationSpec{
			Source: corev1alpha1.ApplicationSource{
				RepositoryRef: corev1alpha1.LocalObjectReference{Name: "platform"},
				Revision:      "main",
				Path:          "apps/payments",
				Render:        corev1alpha1.RenderSpec{Type: renderType},
			},
			Destination: corev1alpha1.ApplicationDestination{Namespace: "payments"},
			Sync: corev1alpha1.SyncPolicy{
				Automatic:      false,
				Prune:          true,
				SelfHeal:       true,
				ConflictPolicy: corev1alpha1.ConflictPolicyFail,
			},
		},
	}
}

func ensureNamespace(ctx context.Context, name string) {
	err := k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}})
	if client.IgnoreAlreadyExists(err) != nil {
		Expect(err).NotTo(HaveOccurred())
	}
}

func createRepository(ctx context.Context) {
	Expect(k8sClient.Create(ctx, &corev1alpha1.Repository{
		ObjectMeta: metav1.ObjectMeta{Name: "platform", Namespace: "default"},
		Spec: corev1alpha1.RepositorySpec{
			Type: corev1alpha1.RepositoryTypeGit,
			Git:  &corev1alpha1.GitRepositorySpec{URL: "https://example.com/platform.git", Revision: "main"},
		},
	})).To(Succeed())
}

func configMapObject(namespace, value string) unstructured.Unstructured {
	obj := unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]any{
			"name": "app-config",
		},
		"data": map[string]any{"key": value},
	}}
	obj.SetNamespace(namespace)
	return obj
}

func secretObject(name string) unstructured.Unstructured {
	return unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Secret",
		"metadata": map[string]any{
			"name": name,
		},
		"stringData": map[string]any{"password": "super-secret"},
	}}
}

func listApplicationRevisions(ctx context.Context, appName string) corev1alpha1.RevisionList {
	list := &corev1alpha1.RevisionList{}
	Expect(k8sClient.List(ctx, list, client.InNamespace("default"), client.MatchingLabels{"solder.io/application": appName})).To(Succeed())
	return *list
}

func deleteApplicationRevisions(ctx context.Context, appName string) {
	list := listApplicationRevisions(ctx, appName)
	for i := range list.Items {
		deleteObject(ctx, &list.Items[i])
	}
}

func deleteObject(ctx context.Context, obj client.Object) {
	key := client.ObjectKeyFromObject(obj)
	if err := k8sClient.Get(ctx, key, obj); err == nil && len(obj.GetFinalizers()) > 0 {
		obj.SetFinalizers(nil)
		Expect(k8sClient.Update(ctx, obj)).To(Succeed())
	}
	err := k8sClient.Delete(ctx, obj)
	if client.IgnoreNotFound(err) != nil {
		Expect(err).NotTo(HaveOccurred())
	}
}

func jsonMarshal(v any) (string, error) {
	data, err := json.Marshal(v)
	return string(data), err
}

// approve records an approval the way the admission webhook does.
func approve(ctx context.Context, key types.NamespacedName, revision corev1alpha1.Revision, user string) {
	app := &corev1alpha1.Application{}
	Expect(k8sClient.Get(ctx, key, app)).To(Succeed())
	app.SetAnnotations(map[string]string{
		corev1alpha1.ApprovedRevisionAnnotation: revision.Name,
		corev1alpha1.ApprovedByAnnotation:       user,
		corev1alpha1.ApprovedAtAnnotation:       time.Now().UTC().Format(time.RFC3339),
		corev1alpha1.ApprovedDigestAnnotation:   revision.Status.Plan.Digest,
	})
	Expect(k8sClient.Update(ctx, app)).To(Succeed())
}
