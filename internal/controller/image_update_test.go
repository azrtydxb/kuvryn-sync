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
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport/client"
	"github.com/go-git/go-git/v5/plumbing/transport/server"
	corev1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1alpha1 "github.com/azrtydxb/kuvryn-sync/api/v1alpha1"
	"github.com/azrtydxb/kuvryn-sync/internal/imageupdate"
	"github.com/azrtydxb/kuvryn-sync/internal/source"
)

func init() {
	// Serve file:// in process so the write-back test needs no git binary.
	client.InstallProtocol("file", server.DefaultServer)
}

var _ = Describe("Image write-back", func() {
	ctx := context.Background()
	key := types.NamespacedName{Name: "images-repo", Namespace: "default"}
	var bare string

	BeforeEach(func() {
		seed := GinkgoT().TempDir()
		repo, err := gogit.PlainInitWithOptions(seed, &gogit.PlainInitOptions{InitOptions: gogit.InitOptions{DefaultBranch: plumbing.NewBranchReferenceName("main")}})
		Expect(err).NotTo(HaveOccurred())
		Expect(os.MkdirAll(filepath.Join(seed, "apps"), 0o700)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(seed, "apps", "deploy.yaml"), []byte("image: ghcr.io/acme/api:1.0.0 # {\"$imagepolicy\": \"default:api\"}\n"), 0o600)).To(Succeed())
		worktree, err := repo.Worktree()
		Expect(err).NotTo(HaveOccurred())
		_, err = worktree.Add("apps/deploy.yaml")
		Expect(err).NotTo(HaveOccurred())
		_, err = worktree.Commit("seed", &gogit.CommitOptions{Author: &object.Signature{Name: "t", Email: "t@example.com", When: time.Now()}})
		Expect(err).NotTo(HaveOccurred())
		bare = filepath.Join(GinkgoT().TempDir(), "origin.git")
		_, err = gogit.PlainClone(bare, true, &gogit.CloneOptions{URL: filepath.Join(seed, ".git")})
		Expect(err).NotTo(HaveOccurred())

		Expect(k8sClient.Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "writer", Namespace: "default", Labels: map[string]string{GitCredentialsLabel: "true"}},
			Data:       map[string][]byte{"username": []byte("bot"), "password": []byte("unused-for-local")},
		})).To(Succeed())
		policy := &corev1alpha1.ImagePolicy{
			ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "default"},
			Spec:       corev1alpha1.ImagePolicySpec{Image: "ghcr.io/acme/api", Policy: corev1alpha1.ImageSelectionPolicy{Semver: &corev1alpha1.SemverPolicy{Range: "^1"}}},
		}
		Expect(k8sClient.Create(ctx, policy)).To(Succeed())
		policy.Status.LatestTag, policy.Status.LatestImage = "1.1.0", "ghcr.io/acme/api:1.1.0@sha256:2222"
		Expect(k8sClient.Status().Update(ctx, policy)).To(Succeed())
		Expect(k8sClient.Create(ctx, &corev1alpha1.Repository{
			ObjectMeta: metav1.ObjectMeta{Name: key.Name, Namespace: "default"},
			Spec: corev1alpha1.RepositorySpec{
				Type:        corev1alpha1.RepositoryTypeGit,
				Git:         &corev1alpha1.GitRepositorySpec{URL: bare, Revision: "main"},
				ImageUpdate: &corev1alpha1.ImageUpdateSpec{SecretRef: corev1alpha1.SecretReference{Name: "writer"}, AuthorName: "Solder", AuthorEmail: "solder@localhost"},
			},
		})).To(Succeed())
	})

	AfterEach(func() {
		deleteObject(ctx, &corev1alpha1.Repository{ObjectMeta: metav1.ObjectMeta{Name: key.Name, Namespace: "default"}})
		deleteObject(ctx, &corev1alpha1.ImagePolicy{ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "default"}})
		deleteObject(ctx, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "writer", Namespace: "default"}})
	})

	It("reports image write-back as disabled when no updater is configured", func() {
		reconciler := &RepositoryReconciler{
			Client: k8sClient, Scheme: k8sClient.Scheme(),
			SourceResolver: &recordingSourceResolver{resolved: source.ResolvedSource{Revision: "abc", CacheDir: GinkgoT().TempDir()}},
		}
		_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: key})
		Expect(err).NotTo(HaveOccurred())
		updated := &corev1alpha1.Repository{}
		Expect(k8sClient.Get(ctx, key, updated)).To(Succeed())
		condition := apimeta.FindStatusCondition(updated.Status.Conditions, "ImagesUpdated")
		Expect(condition).NotTo(BeNil(), "spec.imageUpdate was ignored without a trace")
		Expect(condition.Status).To(Equal(metav1.ConditionFalse))
		Expect(condition.Reason).To(Equal("Disabled"))
	})

	It("commits the selected image to Git once and reports it", func() {
		reconciler := &RepositoryReconciler{
			Client: k8sClient, Scheme: k8sClient.Scheme(),
			SourceResolver: &recordingSourceResolver{resolved: source.ResolvedSource{Revision: "abc", CacheDir: GinkgoT().TempDir()}},
			ImageUpdater:   &imageupdate.Updater{AllowLocal: true},
		}
		result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: key})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(Equal(time.Second))

		repo, err := gogit.PlainOpen(bare)
		Expect(err).NotTo(HaveOccurred())
		head, err := repo.Reference(plumbing.NewBranchReferenceName("main"), true)
		Expect(err).NotTo(HaveOccurred())
		commit, err := repo.CommitObject(head.Hash())
		Expect(err).NotTo(HaveOccurred())
		file, err := commit.File("apps/deploy.yaml")
		Expect(err).NotTo(HaveOccurred())
		Expect(file.Contents()).To(ContainSubstring("image: ghcr.io/acme/api:1.1.0@sha256:2222 #"))

		updated := &corev1alpha1.Repository{}
		Expect(k8sClient.Get(ctx, key, updated)).To(Succeed())
		condition := apimeta.FindStatusCondition(updated.Status.Conditions, "ImagesUpdated")
		Expect(condition).NotTo(BeNil())
		Expect(condition.Reason).To(Equal("Committed"))
		Expect(condition.Message).To(ContainSubstring(head.Hash().String()))

		_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: key})
		Expect(err).NotTo(HaveOccurred())
		again, err := repo.Reference(plumbing.NewBranchReferenceName("main"), true)
		Expect(err).NotTo(HaveOccurred())
		Expect(again.Hash()).To(Equal(head.Hash()), "an unchanged selection was committed again")
		Expect(k8sClient.Get(ctx, key, updated)).To(Succeed())
		Expect(apimeta.FindStatusCondition(updated.Status.Conditions, "ImagesUpdated").Reason).To(Equal("UpToDate"))
	})
})
