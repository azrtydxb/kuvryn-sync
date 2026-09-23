package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	corev1alpha1 "github.com/azrtydxb/solder/api/v1alpha1"
)

func approvalClient(t *testing.T, digest string) client.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := corev1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		&corev1alpha1.Application{ObjectMeta: metav1.ObjectMeta{Name: "payments", Namespace: "default", Annotations: map[string]string{"keep": "me"}}},
		&corev1alpha1.Revision{
			ObjectMeta: metav1.ObjectMeta{Name: "payments-abc", Namespace: "default"},
			Spec:       corev1alpha1.RevisionSpec{ApplicationRef: corev1alpha1.LocalObjectReference{Name: "payments"}},
			Status:     corev1alpha1.RevisionStatus{Plan: corev1alpha1.RevisionPlan{Digest: digest}},
		},
		&corev1alpha1.Revision{
			ObjectMeta: metav1.ObjectMeta{Name: "search-abc", Namespace: "default"},
			Spec:       corev1alpha1.RevisionSpec{ApplicationRef: corev1alpha1.LocalObjectReference{Name: "search"}},
			Status:     corev1alpha1.RevisionStatus{Plan: corev1alpha1.RevisionPlan{Digest: digest}},
		},
	).Build()
}

func TestApproveSendsTheDigestShownToTheApprover(t *testing.T) {
	c := approvalClient(t, "digest-reviewed")
	var stdout bytes.Buffer
	if err := approve(context.Background(), c, "default", "payments", "payments-abc", &stdout); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "digest-reviewed") {
		t.Fatalf("the approver was not shown the plan digest: %s", stdout.String())
	}
	app := &corev1alpha1.Application{}
	if err := c.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "payments"}, app); err != nil {
		t.Fatal(err)
	}
	got := app.GetAnnotations()
	if got[corev1alpha1.ApprovedRevisionAnnotation] != "payments-abc" || got[corev1alpha1.ApproveDigestAnnotation] != "digest-reviewed" || got["keep"] != "me" {
		t.Fatalf("annotations = %v", got)
	}
}

func TestApproveRefusesRevisionsItCannotApprove(t *testing.T) {
	for name, tc := range map[string]struct{ digest, revision string }{
		"no plan yet":         {"", "payments-abc"},
		"another application": {"digest-reviewed", "search-abc"},
		"missing revision":    {"digest-reviewed", "payments-missing"},
	} {
		t.Run(name, func(t *testing.T) {
			c := approvalClient(t, tc.digest)
			if err := approve(context.Background(), c, "default", "payments", tc.revision, &bytes.Buffer{}); err == nil {
				t.Fatal("approval was sent")
			}
			app := &corev1alpha1.Application{}
			if err := c.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "payments"}, app); err != nil {
				t.Fatal(err)
			}
			if _, ok := app.GetAnnotations()[corev1alpha1.ApprovedRevisionAnnotation]; ok {
				t.Fatal("approval was recorded")
			}
		})
	}
}
