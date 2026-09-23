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
	"os"
	"path/filepath"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1alpha1 "github.com/azrtydxb/solder/api/v1alpha1"
	"github.com/azrtydxb/solder/internal/decrypt/decrypttest"
	"github.com/azrtydxb/solder/internal/renderer"
	yamlrenderer "github.com/azrtydxb/solder/internal/renderer/yaml"
	"github.com/azrtydxb/solder/internal/source"
)

type workspaceResolver string

func (w workspaceResolver) Resolve(context.Context, source.GitRepository) (source.ResolvedSource, error) {
	return source.ResolvedSource{Revision: "resolved-sha", CacheDir: string(w)}, nil
}

var _ = Describe("SOPS decryption", func() {
	const appName = "sops-app"
	ctx := context.Background()
	key := types.NamespacedName{Name: appName, Namespace: "default"}
	secretKey := types.NamespacedName{Name: "db", Namespace: "payments"}
	var reconciler *ApplicationReconciler

	BeforeEach(func() {
		ensureNamespace(ctx, "payments")
		createRepository(ctx)
		age := decrypttest.Identity(GinkgoT())
		workspace := GinkgoT().TempDir()
		Expect(os.MkdirAll(filepath.Join(workspace, "apps", "payments"), 0o700)).To(Succeed())
		encrypted := decrypttest.Encrypt(GinkgoT(), age.Recipient().String(), "apiVersion: v1\nkind: Secret\nmetadata:\n  name: db\nstringData:\n  password: hunter2\n")
		Expect(os.WriteFile(filepath.Join(workspace, "apps", "payments", "secret.yaml"), encrypted, 0o600)).To(Succeed())
		Expect(k8sClient.Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "sops-age", Namespace: "default", Labels: map[string]string{DecryptionKeyLabel: "true"}},
			Data:       map[string][]byte{"payments.agekey": []byte(age.String())},
		})).To(Succeed())

		reconciler = newApplicationReconciler(nil, nil)
		reconciler.SourceResolver = workspaceResolver(workspace)
		reconciler.Renderers = func(corev1alpha1.RenderType) (renderer.Renderer, error) { return yamlrenderer.Renderer{}, nil }
	})

	AfterEach(func() {
		deleteObject(ctx, &corev1alpha1.Application{ObjectMeta: metav1.ObjectMeta{Name: appName, Namespace: "default"}})
		deleteObject(ctx, &corev1alpha1.Repository{ObjectMeta: metav1.ObjectMeta{Name: "platform", Namespace: "default"}})
		deleteObject(ctx, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "sops-age", Namespace: "default"}})
		deleteObject(ctx, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "db", Namespace: "payments"}})
		deleteApplicationRevisions(ctx, appName)
	})

	It("applies the decrypted Secret and never shows plaintext in the plan", func() {
		app := newApplication(appName, corev1alpha1.RenderTypeYAML)
		app.Spec.Sync.Automatic = true
		app.Spec.Decryption = &corev1alpha1.DecryptionSpec{Provider: "sops", SecretRef: corev1alpha1.SecretReference{Name: "sops-age"}}
		Expect(k8sClient.Create(ctx, app)).To(Succeed())
		_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: key})
		Expect(err).NotTo(HaveOccurred())

		live := &corev1.Secret{}
		Expect(k8sClient.Get(ctx, secretKey, live)).To(Succeed())
		Expect(string(live.Data["password"])).To(Equal("hunter2"))
		plan, err := json.Marshal(listApplicationRevisions(ctx, appName).Items[0].Status)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(plan)).NotTo(ContainSubstring("hunter2"))
	})

	It("refuses to apply ciphertext when decryption is not configured", func() {
		app := newApplication(appName, corev1alpha1.RenderTypeYAML)
		app.Spec.Sync.Automatic = true
		Expect(k8sClient.Create(ctx, app)).To(Succeed())
		_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: key})
		Expect(err).NotTo(HaveOccurred())

		failure := listApplicationRevisions(ctx, appName).Items[0].Status.Failure
		Expect(failure).NotTo(BeNil())
		Expect(failure.Message).To(ContainSubstring("spec.decryption"))
		Expect(apierrors.IsNotFound(k8sClient.Get(ctx, secretKey, &corev1.Secret{}))).To(BeTrue())
	})
})
