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
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	corev1alpha1 "github.com/azrtydxb/kuvryn-sync/api/v1alpha1"
	"github.com/azrtydxb/kuvryn-sync/internal/source"
)

type recordingSourceResolver struct {
	repository source.GitRepository
	resolved   source.ResolvedSource
	err        error
}

func (r *recordingSourceResolver) Resolve(_ context.Context, repository source.GitRepository) (source.ResolvedSource, error) {
	r.repository = repository
	if r.err != nil {
		return source.ResolvedSource{}, r.err
	}
	return r.resolved, nil
}

var _ = Describe("Repository Controller", func() {
	const resourceName = "test-repository"

	ctx := context.Background()
	typeNamespacedName := types.NamespacedName{Name: resourceName, Namespace: "default"}

	AfterEach(func() {
		apps := &corev1alpha1.ApplicationList{}
		Expect(k8sClient.List(ctx, apps)).To(Succeed())
		for i := range apps.Items {
			Expect(k8sClient.Delete(ctx, &apps.Items[i])).To(Succeed())
		}
		resource := &corev1alpha1.Repository{}
		if err := k8sClient.Get(ctx, typeNamespacedName, resource); err == nil {
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		}
		secret := &corev1.Secret{}
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: "platform-git", Namespace: "default"}, secret); err == nil {
			Expect(k8sClient.Delete(ctx, secret)).To(Succeed())
		}
	})

	It("resolves a Git source and records readiness without exposing credentials", func() {
		Expect(k8sClient.Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "platform-git", Namespace: "default", Labels: map[string]string{GitCredentialsLabel: "true"}},
			Data: map[string][]byte{
				"username": []byte("git-user"),
				"token":    []byte("super-secret-token"),
			},
		})).To(Succeed())

		resource := &corev1alpha1.Repository{
			ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: "default"},
			Spec: corev1alpha1.RepositorySpec{
				Type: corev1alpha1.RepositoryTypeGit,
				Git: &corev1alpha1.GitRepositorySpec{
					URL:      "https://example.com/acme/platform.git",
					Revision: "main",
					Auth: &corev1alpha1.GitAuthSpec{
						SecretRef: &corev1alpha1.SecretReference{Name: "platform-git"},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, resource)).To(Succeed())

		resolver := &recordingSourceResolver{resolved: source.ResolvedSource{Revision: "8c51af2", CacheDir: "/cache/platform"}}
		controllerReconciler := &RepositoryReconciler{Client: k8sClient, Scheme: k8sClient.Scheme(), SourceResolver: resolver}
		_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
		Expect(err).NotTo(HaveOccurred())

		updated := &corev1alpha1.Repository{}
		Expect(k8sClient.Get(ctx, typeNamespacedName, updated)).To(Succeed())
		Expect(updated.Status.State).To(Equal(corev1alpha1.RepositoryStateReady))
		Expect(updated.Status.ObservedRevision).To(Equal("8c51af2"))
		Expect(updated.Status.Conditions).NotTo(BeEmpty())
		Expect(updated.Status.Conditions[0].Message).NotTo(ContainSubstring("super-secret-token"))
		Expect(resolver.repository.Auth.Token).To(Equal("super-secret-token"))
	})

	It("discovers multiple Applications from a root .ksync.yaml file", func() {
		workspace := GinkgoT().TempDir()
		Expect(os.WriteFile(filepath.Join(workspace, ".ksync.yaml"), []byte(`applications:
- metadata:
    name: payments
  spec:
    source:
      path: apps/payments
      render:
        type: kustomize
    destination:
      namespace: payments
    sync:
      automatic: true
      prune: true
      conflictPolicy: fail
- metadata:
    name: search
  spec:
    source:
      path: apps/search
      render:
        type: yaml
    destination:
      namespace: search
    sync:
      automatic: true
      conflictPolicy: fail
`), 0o600)).To(Succeed())

		resource := &corev1alpha1.Repository{
			ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: "default"},
			Spec: corev1alpha1.RepositorySpec{
				Type: corev1alpha1.RepositoryTypeGit,
				Git:  &corev1alpha1.GitRepositorySpec{URL: "https://example.com/acme/platform.git", Revision: "main"},
			},
		}
		Expect(k8sClient.Create(ctx, resource)).To(Succeed())

		resolver := &recordingSourceResolver{resolved: source.ResolvedSource{Revision: "8c51af2", CacheDir: workspace}}
		controllerReconciler := &RepositoryReconciler{Client: k8sClient, Scheme: k8sClient.Scheme(), SourceResolver: resolver}
		_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
		Expect(err).NotTo(HaveOccurred())

		app := &corev1alpha1.Application{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "payments", Namespace: "default"}, app)).To(Succeed())
		Expect(app.Spec.Source.RepositoryRef.Name).To(Equal(resourceName))
		Expect(app.Spec.Source.Path).To(Equal("apps/payments"))
		Expect(app.Spec.Source.Render.Type).To(Equal(corev1alpha1.RenderTypeKustomize))
		Expect(app.Spec.Destination.Namespace).To(Equal("payments"))
		Expect(app.Labels[repositoryApplicationLabel]).To(Equal(resourceName))
		Expect(app.OwnerReferences).NotTo(BeEmpty())
		search := &corev1alpha1.Application{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "search", Namespace: "default"}, search)).To(Succeed())
		Expect(search.Spec.Source.RepositoryRef.Name).To(Equal(resourceName))
		Expect(search.Spec.Source.Path).To(Equal("apps/search"))
		Expect(search.Spec.Source.Render.Type).To(Equal(corev1alpha1.RenderTypeYAML))
	})

	It("discovers Applications from configured .ksync.yaml paths", func() {
		workspace := GinkgoT().TempDir()
		Expect(os.MkdirAll(filepath.Join(workspace, "teams/payments"), 0o700)).To(Succeed())
		Expect(os.MkdirAll(filepath.Join(workspace, "teams/search"), 0o700)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(workspace, "teams/payments/.ksync.yaml"), []byte(`applications:
- metadata:
    name: payments
  spec:
    source:
      path: teams/payments/deploy
      render:
        type: yaml
    destination:
      namespace: payments
`), 0o600)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(workspace, "teams/search/.ksync.yaml"), []byte(`applications:
- metadata:
    name: search
  spec:
    source:
      path: teams/search/deploy
      render:
        type: kustomize
    destination:
      namespace: search
`), 0o600)).To(Succeed())

		resource := &corev1alpha1.Repository{
			ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: "default"},
			Spec: corev1alpha1.RepositorySpec{
				Type:                   corev1alpha1.RepositoryTypeGit,
				Git:                    &corev1alpha1.GitRepositorySpec{URL: "https://example.com/acme/platform.git", Revision: "main"},
				ApplicationConfigPaths: []string{"teams/payments/.ksync.yaml", "teams/search/.ksync.yaml"},
			},
		}
		Expect(k8sClient.Create(ctx, resource)).To(Succeed())

		resolver := &recordingSourceResolver{resolved: source.ResolvedSource{Revision: "8c51af2", CacheDir: workspace}}
		controllerReconciler := &RepositoryReconciler{Client: k8sClient, Scheme: k8sClient.Scheme(), SourceResolver: resolver}
		_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
		Expect(err).NotTo(HaveOccurred())

		payments := &corev1alpha1.Application{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "payments", Namespace: "default"}, payments)).To(Succeed())
		Expect(payments.Annotations["sync.kuvryn.io/discovered-from"]).To(Equal("teams/payments/.ksync.yaml"))
		search := &corev1alpha1.Application{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "search", Namespace: "default"}, search)).To(Succeed())
		Expect(search.Spec.Source.RepositoryRef.Name).To(Equal(resourceName))
		Expect(search.Annotations["sync.kuvryn.io/discovered-from"]).To(Equal("teams/search/.ksync.yaml"))
	})

	It("rejects unsafe configured .ksync.yaml paths", func() {
		resource := &corev1alpha1.Repository{
			ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: "default"},
			Spec: corev1alpha1.RepositorySpec{
				Type:                   corev1alpha1.RepositoryTypeGit,
				Git:                    &corev1alpha1.GitRepositorySpec{URL: "https://example.com/acme/platform.git", Revision: "main"},
				ApplicationConfigPaths: []string{"../.ksync.yaml"},
			},
		}
		Expect(k8sClient.Create(ctx, resource)).To(Succeed())

		resolver := &recordingSourceResolver{resolved: source.ResolvedSource{Revision: "8c51af2", CacheDir: GinkgoT().TempDir()}}
		controllerReconciler := &RepositoryReconciler{Client: k8sClient, Scheme: k8sClient.Scheme(), SourceResolver: resolver}
		_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
		Expect(err).NotTo(HaveOccurred())

		updated := &corev1alpha1.Repository{}
		Expect(k8sClient.Get(ctx, typeNamespacedName, updated)).To(Succeed())
		Expect(updated.Status.State).To(Equal(corev1alpha1.RepositoryStateFailed))
		Expect(updated.Status.Conditions[0].Reason).To(Equal(string(source.FailureReasonValidationFailure)))
	})

	It("prunes Applications removed from .ksync.yaml", func() {
		workspace := GinkgoT().TempDir()
		Expect(os.WriteFile(filepath.Join(workspace, ".ksync.yaml"), []byte(`applications: []
`), 0o600)).To(Succeed())
		resource := &corev1alpha1.Repository{
			ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: "default"},
			Spec: corev1alpha1.RepositorySpec{
				Type: corev1alpha1.RepositoryTypeGit,
				Git:  &corev1alpha1.GitRepositorySpec{URL: "https://example.com/acme/platform.git", Revision: "main"},
			},
		}
		Expect(k8sClient.Create(ctx, resource)).To(Succeed())
		stale := &corev1alpha1.Application{
			ObjectMeta: metav1.ObjectMeta{Name: "old-payments", Namespace: "default", Labels: map[string]string{repositoryApplicationLabel: resourceName}},
			Spec:       corev1alpha1.ApplicationSpec{Source: corev1alpha1.ApplicationSource{RepositoryRef: corev1alpha1.LocalObjectReference{Name: resourceName}, Path: "old", Render: corev1alpha1.RenderSpec{Type: corev1alpha1.RenderTypeYAML}}},
		}
		Expect(k8sClient.Create(ctx, stale)).To(Succeed())

		resolver := &recordingSourceResolver{resolved: source.ResolvedSource{Revision: "8c51af2", CacheDir: workspace}}
		controllerReconciler := &RepositoryReconciler{Client: k8sClient, Scheme: k8sClient.Scheme(), SourceResolver: resolver}
		_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
		Expect(err).NotTo(HaveOccurred())

		deleted := &corev1alpha1.Application{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "old-payments", Namespace: "default"}, deleted)).To(MatchError(ContainSubstring("not found")))
	})

	DescribeTable("constrains the service account of discovered Applications",
		func(pinned, declared, wantAccount, wantError string) {
			workspace := GinkgoT().TempDir()
			serviceAccountLine := ""
			if declared != "" {
				serviceAccountLine = "\n    serviceAccountName: " + declared
			}
			Expect(os.WriteFile(filepath.Join(workspace, ".ksync.yaml"), []byte(`applications:
- metadata:
    name: payments
  spec:`+serviceAccountLine+`
    source:
      path: apps/payments
      render:
        type: yaml
    destination:
      namespace: payments
`), 0o600)).To(Succeed())
			Expect(k8sClient.Create(ctx, &corev1alpha1.Repository{
				ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: "default"},
				Spec: corev1alpha1.RepositorySpec{
					Type:                          corev1alpha1.RepositoryTypeGit,
					Git:                           &corev1alpha1.GitRepositorySpec{URL: "https://example.com/acme/platform.git", Revision: "main"},
					ApplicationServiceAccountName: pinned,
				},
			})).To(Succeed())

			resolver := &recordingSourceResolver{resolved: source.ResolvedSource{Revision: "8c51af2", CacheDir: workspace}}
			controllerReconciler := &RepositoryReconciler{Client: k8sClient, Scheme: k8sClient.Scheme(), SourceResolver: resolver}
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			updated := &corev1alpha1.Repository{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, updated)).To(Succeed())
			app := &corev1alpha1.Application{}
			appErr := k8sClient.Get(ctx, types.NamespacedName{Name: "payments", Namespace: "default"}, app)
			if wantError != "" {
				Expect(updated.Status.State).To(Equal(corev1alpha1.RepositoryStateFailed))
				Expect(updated.Status.Conditions[0].Message).To(ContainSubstring(wantError))
				Expect(apierrors.IsNotFound(appErr)).To(BeTrue(), "escalating Application was created")
				return
			}
			Expect(appErr).NotTo(HaveOccurred())
			Expect(app.Spec.ServiceAccountName).To(Equal(wantAccount))
		},
		Entry("inherits the pinned account", "payments-deployer", "", "payments-deployer", ""),
		Entry("accepts naming the pinned account", "payments-deployer", "payments-deployer", "payments-deployer", ""),
		Entry("uses the controller default when nothing is pinned", "", "", "", ""),
		Entry("rejects a different account than the pinned one", "payments-deployer", "cluster-operator", "", `pins "payments-deployer"`),
		Entry("rejects any account when nothing is pinned", "", "cluster-operator", "", "may not set serviceAccountName"),
	)

	It("reports invalid .ksync.yaml files as validation failures", func() {
		workspace := GinkgoT().TempDir()
		Expect(os.WriteFile(filepath.Join(workspace, ".ksync.yaml"), []byte(`applications:
- metadata:
    name: broken
  spec:
    source:
      path: apps/broken
`), 0o600)).To(Succeed())

		resource := &corev1alpha1.Repository{
			ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: "default"},
			Spec: corev1alpha1.RepositorySpec{
				Type: corev1alpha1.RepositoryTypeGit,
				Git:  &corev1alpha1.GitRepositorySpec{URL: "https://example.com/acme/platform.git", Revision: "main"},
			},
		}
		Expect(k8sClient.Create(ctx, resource)).To(Succeed())

		resolver := &recordingSourceResolver{resolved: source.ResolvedSource{Revision: "8c51af2", CacheDir: workspace}}
		controllerReconciler := &RepositoryReconciler{Client: k8sClient, Scheme: k8sClient.Scheme(), SourceResolver: resolver}
		_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
		Expect(err).NotTo(HaveOccurred())

		updated := &corev1alpha1.Repository{}
		Expect(k8sClient.Get(ctx, typeNamespacedName, updated)).To(Succeed())
		Expect(updated.Status.State).To(Equal(corev1alpha1.RepositoryStateFailed))
		Expect(updated.Status.Conditions[0].Reason).To(Equal(string(source.FailureReasonValidationFailure)))
	})

	It("refuses to send an unlabelled Secret as Git credentials", func() {
		Expect(k8sClient.Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "platform-git", Namespace: "default"},
			Data:       map[string][]byte{"password": []byte("database-password")},
		})).To(Succeed())
		resource := &corev1alpha1.Repository{
			ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: "default"},
			Spec: corev1alpha1.RepositorySpec{
				Type: corev1alpha1.RepositoryTypeGit,
				Git: &corev1alpha1.GitRepositorySpec{
					URL:      "https://attacker.example/x.git",
					Revision: "main",
					Auth:     &corev1alpha1.GitAuthSpec{SecretRef: &corev1alpha1.SecretReference{Name: "platform-git"}},
				},
			},
		}
		Expect(k8sClient.Create(ctx, resource)).To(Succeed())

		resolver := &recordingSourceResolver{}
		controllerReconciler := &RepositoryReconciler{Client: k8sClient, Scheme: k8sClient.Scheme(), SourceResolver: resolver}
		_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
		Expect(err).NotTo(HaveOccurred())

		updated := &corev1alpha1.Repository{}
		Expect(k8sClient.Get(ctx, typeNamespacedName, updated)).To(Succeed())
		Expect(updated.Status.State).To(Equal(corev1alpha1.RepositoryStateFailed))
		Expect(updated.Status.Conditions[0].Reason).To(Equal(string(source.FailureReasonAuthenticationFailure)))
		Expect(updated.Status.Conditions[0].Message).To(ContainSubstring(GitCredentialsLabel))
		Expect(resolver.repository.Auth.Password).To(BeEmpty(), "unlabelled Secret was sent to the Git remote")
	})

	It("reports missing auth Secrets as authentication failures", func() {
		resource := &corev1alpha1.Repository{
			ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: "default"},
			Spec: corev1alpha1.RepositorySpec{
				Type: corev1alpha1.RepositoryTypeGit,
				Git: &corev1alpha1.GitRepositorySpec{
					URL:      "ssh://git@example.com/acme/platform.git",
					Revision: "main",
					Auth: &corev1alpha1.GitAuthSpec{
						SecretRef: &corev1alpha1.SecretReference{Name: "platform-git"},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, resource)).To(Succeed())

		controllerReconciler := &RepositoryReconciler{Client: k8sClient, Scheme: k8sClient.Scheme(), SourceResolver: &recordingSourceResolver{}}
		_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
		Expect(err).NotTo(HaveOccurred())

		updated := &corev1alpha1.Repository{}
		Expect(k8sClient.Get(ctx, typeNamespacedName, updated)).To(Succeed())
		Expect(updated.Status.State).To(Equal(corev1alpha1.RepositoryStateFailed))
		condition := updated.Status.Conditions[0]
		Expect(condition.Status).To(Equal(metav1.ConditionFalse))
		Expect(condition.Reason).To(Equal(string(source.FailureReasonAuthenticationFailure)))
	})
})
