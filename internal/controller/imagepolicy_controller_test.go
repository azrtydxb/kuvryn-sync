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
	"encoding/base64"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	corev1alpha1 "github.com/azrtydxb/solder/api/v1alpha1"
	"github.com/azrtydxb/solder/internal/imagepolicy"
	"github.com/azrtydxb/solder/internal/imagepolicy/registrytest"
)

const (
	digestOne = "sha256:1111111111111111111111111111111111111111111111111111111111111111"
	digestTwo = "sha256:2222222222222222222222222222222222222222222222222222222222222222"
)

var _ = Describe("ImagePolicy Controller", func() {
	ctx := context.Background()
	key := types.NamespacedName{Name: "api", Namespace: "default"}
	var registry *registrytest.Registry
	var reconciler *ImagePolicyReconciler

	BeforeEach(func() {
		registry = registrytest.New("robot", "pw")
		registry.Push("acme/api", "1.0.0", digestOne)
		reconciler = &ImagePolicyReconciler{Client: k8sClient, Scheme: k8sClient.Scheme(), Registry: &imagepolicy.Registry{PlainHTTP: true}}
	})

	AfterEach(func() {
		registry.Close()
		deleteObject(ctx, &corev1alpha1.ImagePolicy{ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "default"}})
		deleteObject(ctx, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "registry", Namespace: "default"}})
	})

	createPolicy := func(labelled bool) {
		labels := map[string]string{}
		if labelled {
			labels[RegistryCredentialsLabel] = "true"
		}
		config := `{"auths":{"` + registry.Host() + `":{"auth":"` + base64.StdEncoding.EncodeToString([]byte("robot:pw")) + `"}}}`
		Expect(k8sClient.Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "registry", Namespace: "default", Labels: labels},
			Type:       corev1.SecretTypeDockerConfigJson,
			Data:       map[string][]byte{corev1.DockerConfigJsonKey: []byte(config)},
		})).To(Succeed())
		Expect(k8sClient.Create(ctx, &corev1alpha1.ImagePolicy{
			ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "default"},
			Spec: corev1alpha1.ImagePolicySpec{
				Image:     registry.Host() + "/acme/api",
				SecretRef: &corev1alpha1.SecretReference{Name: "registry"},
				Interval:  metav1.Duration{Duration: time.Minute},
				Policy:    corev1alpha1.ImageSelectionPolicy{Semver: &corev1alpha1.SemverPolicy{Range: "^1.0.0"}},
			},
		})).To(Succeed())
	}

	It("selects the newest matching tag by digest and picks up a newly pushed build", func() {
		createPolicy(true)
		result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: key})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(Equal(time.Minute))
		policy := &corev1alpha1.ImagePolicy{}
		Expect(k8sClient.Get(ctx, key, policy)).To(Succeed())
		Expect(policy.Status.LatestImage).To(Equal(registry.Host() + "/acme/api:1.0.0@" + digestOne))
		Expect(policy.Status.LastScannedAt).NotTo(BeNil())

		registry.Push("acme/api", "1.1.0", digestTwo)
		_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: key})
		Expect(err).NotTo(HaveOccurred())
		Expect(k8sClient.Get(ctx, key, policy)).To(Succeed())
		Expect(policy.Status.LatestTag).To(Equal("1.1.0"))
		Expect(policy.Status.LatestDigest).To(Equal(digestTwo))
	})

	It("refuses registry credentials that are not labelled for registries", func() {
		createPolicy(false)
		_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: key})
		Expect(err).NotTo(HaveOccurred())
		policy := &corev1alpha1.ImagePolicy{}
		Expect(k8sClient.Get(ctx, key, policy)).To(Succeed())
		ready := apimeta.FindStatusCondition(policy.Status.Conditions, "Ready")
		Expect(ready).NotTo(BeNil())
		Expect(ready.Reason).To(Equal("ScanFailed"))
		Expect(ready.Message).To(ContainSubstring(RegistryCredentialsLabel))
		Expect(policy.Status.LatestImage).To(BeEmpty())
	})

	It("rejects a policy that sets more than one selector", func() {
		err := k8sClient.Create(ctx, &corev1alpha1.ImagePolicy{
			ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "default"},
			Spec: corev1alpha1.ImagePolicySpec{Image: "ghcr.io/acme/api", Policy: corev1alpha1.ImageSelectionPolicy{
				Semver: &corev1alpha1.SemverPolicy{Range: "^1"}, Digest: &corev1alpha1.DigestPolicy{Tag: "main"},
			}},
		})
		Expect(apierrors.IsInvalid(err)).To(BeTrue(), "err = %v", err)
	})
})
