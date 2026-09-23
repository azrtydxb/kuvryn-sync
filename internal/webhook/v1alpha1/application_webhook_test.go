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

package v1alpha1

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	admissionv1 "k8s.io/api/admission/v1"
	authenticationv1 "k8s.io/api/authentication/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	corev1alpha1 "github.com/azrtydxb/solder/api/v1alpha1"
)

var _ = Describe("Application approval webhook", Ordered, func() {
	const appName = "approval-app"
	var alice client.Client
	key := client.ObjectKey{Name: appName, Namespace: "default"}

	BeforeAll(func() {
		Expect(k8sClient.Create(ctx, &rbacv1.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: "alice-admin"},
			RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: "cluster-admin"},
			Subjects:   []rbacv1.Subject{{Kind: rbacv1.UserKind, APIGroup: rbacv1.GroupName, Name: "alice@example.com"}},
		})).To(Succeed())
		aliceConfig := rest.CopyConfig(cfg)
		aliceConfig.Impersonate = rest.ImpersonationConfig{UserName: "alice@example.com"}
		var err error
		alice, err = client.New(aliceConfig, client.Options{Scheme: scheme.Scheme})
		Expect(err).NotTo(HaveOccurred())

		Expect(k8sClient.Create(ctx, &corev1alpha1.Application{
			ObjectMeta: metav1.ObjectMeta{Name: appName, Namespace: "default"},
			Spec: corev1alpha1.ApplicationSpec{
				Source: corev1alpha1.ApplicationSource{
					RepositoryRef: corev1alpha1.LocalObjectReference{Name: "platform"},
					Render:        corev1alpha1.RenderSpec{Type: corev1alpha1.RenderTypeYAML},
				},
			},
		})).To(Succeed())
		for name, digest := range map[string]string{"approval-app-planned": "digest-reviewed", "approval-app-unplanned": ""} {
			revision := &corev1alpha1.Revision{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
				Spec: corev1alpha1.RevisionSpec{
					ApplicationRef: corev1alpha1.LocalObjectReference{Name: appName},
					Source: corev1alpha1.RevisionSource{
						RepositoryRef: corev1alpha1.LocalObjectReference{Name: "platform"},
						Revision:      "abc123",
						Render:        corev1alpha1.RenderSpec{Type: corev1alpha1.RenderTypeYAML},
					},
				},
			}
			Expect(k8sClient.Create(ctx, revision)).To(Succeed())
			revision.Status.Plan.Digest = digest
			Expect(k8sClient.Status().Update(ctx, revision)).To(Succeed())
		}
	})

	annotate := func(c client.Client, annotations map[string]string) error {
		app := &corev1alpha1.Application{}
		Expect(k8sClient.Get(ctx, key, app)).To(Succeed())
		current := app.GetAnnotations()
		if current == nil {
			current = map[string]string{}
		}
		for k, v := range annotations {
			if v == "" {
				delete(current, k)
			} else {
				current[k] = v
			}
		}
		app.SetAnnotations(current)
		return c.Update(ctx, app)
	}
	annotations := func() map[string]string {
		app := &corev1alpha1.Application{}
		Expect(k8sClient.Get(ctx, key, app)).To(Succeed())
		return app.GetAnnotations()
	}

	It("records the authenticated approver and the plan digest they approved", func() {
		Expect(annotate(alice, map[string]string{corev1alpha1.ApprovedRevisionAnnotation: "approval-app-planned"})).To(Succeed())
		got := annotations()
		Expect(got).To(HaveKeyWithValue(corev1alpha1.ApprovedByAnnotation, "alice@example.com"))
		Expect(got).To(HaveKeyWithValue(corev1alpha1.ApprovedDigestAnnotation, "digest-reviewed"))
		Expect(got).To(HaveKey(corev1alpha1.ApprovedAtAnnotation))
	})

	It("reverts forged approval records", func() {
		before := annotations()
		Expect(annotate(k8sClient, map[string]string{
			corev1alpha1.ApprovedByAnnotation:     "mallory@example.com",
			corev1alpha1.ApprovedDigestAnnotation: "digest-forged",
		})).To(Succeed())
		after := annotations()
		Expect(after[corev1alpha1.ApprovedByAnnotation]).To(Equal(before[corev1alpha1.ApprovedByAnnotation]))
		Expect(after[corev1alpha1.ApprovedDigestAnnotation]).To(Equal("digest-reviewed"))
	})

	It("rejects approving a Revision that has no plan", func() {
		err := annotate(alice, map[string]string{corev1alpha1.ApprovedRevisionAnnotation: "approval-app-unplanned"})
		Expect(apierrors.IsBadRequest(err)).To(BeTrue(), "err = %v", err)
	})

	It("clears the record when the approval is withdrawn", func() {
		Expect(annotate(alice, map[string]string{corev1alpha1.ApprovedRevisionAnnotation: ""})).To(Succeed())
		got := annotations()
		for _, k := range []string{corev1alpha1.ApprovedByAnnotation, corev1alpha1.ApprovedAtAnnotation, corev1alpha1.ApprovedDigestAnnotation} {
			Expect(got).NotTo(HaveKey(k))
		}
	})

	setPlanDigest := func(name, digest string) {
		revision := &corev1alpha1.Revision{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: "default"}, revision)).To(Succeed())
		revision.Status.Plan.Digest = digest
		Expect(k8sClient.Status().Update(ctx, revision)).To(Succeed())
	}

	It("re-stamps a stale approval when the same Revision is approved again with the digest reviewed", func() {
		Expect(annotate(alice, map[string]string{
			corev1alpha1.ApprovedRevisionAnnotation: "approval-app-planned",
			corev1alpha1.ApproveDigestAnnotation:    "digest-reviewed",
		})).To(Succeed())
		Expect(annotations()).To(HaveKeyWithValue(corev1alpha1.ApprovedDigestAnnotation, "digest-reviewed"))

		setPlanDigest("approval-app-planned", "digest-changed")
		Expect(annotate(alice, map[string]string{
			corev1alpha1.ApprovedRevisionAnnotation: "approval-app-planned",
			corev1alpha1.ApproveDigestAnnotation:    "digest-changed",
		})).To(Succeed())
		got := annotations()
		Expect(got).To(HaveKeyWithValue(corev1alpha1.ApprovedDigestAnnotation, "digest-changed"))
		Expect(got).To(HaveKeyWithValue(corev1alpha1.ApprovedByAnnotation, "alice@example.com"))
		Expect(got).NotTo(HaveKey(corev1alpha1.ApproveDigestAnnotation))
	})

	It("rejects an approval of a plan that is no longer current", func() {
		err := annotate(alice, map[string]string{
			corev1alpha1.ApprovedRevisionAnnotation: "approval-app-planned",
			corev1alpha1.ApproveDigestAnnotation:    "digest-reviewed",
		})
		Expect(apierrors.IsBadRequest(err)).To(BeTrue(), "err = %v", err)
		Expect(err.Error()).To(ContainSubstring("digest-changed"))
		Expect(annotations()).To(HaveKeyWithValue(corev1alpha1.ApprovedDigestAnnotation, "digest-changed"))
	})

	It("never turns an unrelated update into a fresh approval", func() {
		before := annotations()
		setPlanDigest("approval-app-planned", "digest-later")
		Expect(annotate(alice, map[string]string{"example.com/note": "unrelated"})).To(Succeed())
		after := annotations()
		Expect(after).To(HaveKeyWithValue(corev1alpha1.ApprovedDigestAnnotation, "digest-changed"))
		Expect(after[corev1alpha1.ApprovedAtAnnotation]).To(Equal(before[corev1alpha1.ApprovedAtAnnotation]))
	})

	It("never stores the approval request, even on create", func() {
		const created = "approval-app-created"
		revision := &corev1alpha1.Revision{
			ObjectMeta: metav1.ObjectMeta{Name: created + "-planned", Namespace: "default"},
			Spec: corev1alpha1.RevisionSpec{
				ApplicationRef: corev1alpha1.LocalObjectReference{Name: created},
				Source: corev1alpha1.RevisionSource{
					RepositoryRef: corev1alpha1.LocalObjectReference{Name: "platform"},
					Revision:      "abc123",
					Render:        corev1alpha1.RenderSpec{Type: corev1alpha1.RenderTypeYAML},
				},
			},
		}
		Expect(k8sClient.Create(ctx, revision)).To(Succeed())
		setPlanDigest(revision.Name, "digest-created")
		app := &corev1alpha1.Application{
			ObjectMeta: metav1.ObjectMeta{Name: created, Namespace: "default", Annotations: map[string]string{
				corev1alpha1.ApprovedRevisionAnnotation: revision.Name,
				corev1alpha1.ApproveDigestAnnotation:    "digest-created",
			}},
			Spec: corev1alpha1.ApplicationSpec{
				Source: corev1alpha1.ApplicationSource{
					RepositoryRef: corev1alpha1.LocalObjectReference{Name: "platform"},
					Render:        corev1alpha1.RenderSpec{Type: corev1alpha1.RenderTypeYAML},
				},
			},
		}
		Expect(alice.Create(ctx, app)).To(Succeed())
		stored := &corev1alpha1.Application{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(app), stored)).To(Succeed())
		Expect(stored.GetAnnotations()).To(HaveKeyWithValue(corev1alpha1.ApprovedDigestAnnotation, "digest-created"))
		Expect(stored.GetAnnotations()).To(HaveKeyWithValue(corev1alpha1.ApprovedByAnnotation, "alice@example.com"))
		Expect(stored.GetAnnotations()).NotTo(HaveKey(corev1alpha1.ApproveDigestAnnotation))
	})
})

var _ = Describe("Application approval webhook without an authenticated user", func() {
	It("rejects the approval", func() {
		defaulter := &ApplicationCustomDefaulter{Reader: k8sClient, Now: time.Now}
		app := &corev1alpha1.Application{ObjectMeta: metav1.ObjectMeta{
			Name: "anonymous", Namespace: "default",
			Annotations: map[string]string{corev1alpha1.ApprovedRevisionAnnotation: "any"},
		}}
		for _, user := range []authenticationv1.UserInfo{
			{},
			{Username: "system:anonymous", Groups: []string{"system:unauthenticated"}},
			{Username: "someone", Groups: []string{"system:unauthenticated"}},
		} {
			anonymous := admission.NewContextWithRequest(ctx, admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{UserInfo: user}})
			err := defaulter.Default(anonymous, app.DeepCopy())
			Expect(apierrors.IsForbidden(err)).To(BeTrue(), "user %+v: err = %v", user, err)
		}
		Expect(app.GetAnnotations()).NotTo(HaveKey(corev1alpha1.ApprovedByAnnotation))
	})
})
