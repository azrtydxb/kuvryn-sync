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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	corev1alpha1 "github.com/azrtydxb/solder/api/v1alpha1"
	"github.com/azrtydxb/solder/internal/source"
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
			ObjectMeta: metav1.ObjectMeta{Name: "platform-git", Namespace: "default"},
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
