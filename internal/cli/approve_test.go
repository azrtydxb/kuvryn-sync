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
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

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
	var patches []string
	c := interceptor.NewClient(approvalClient(t, "digest-reviewed").(client.WithWatch), interceptor.Funcs{
		Patch: func(ctx context.Context, c client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
			body, err := patch.Data(obj)
			if err != nil {
				return err
			}
			patches = append(patches, string(body))
			return c.Patch(ctx, obj, patch, opts...)
		},
	})
	var stdout bytes.Buffer
	if err := approve(context.Background(), c, "default", "payments", "payments-abc", &stdout); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "digest-reviewed") {
		t.Fatalf("the approver was not shown the plan digest: %s", stdout.String())
	}
	if len(patches) != 1 {
		t.Fatalf("sent %d patches, want both annotations in one: %v", len(patches), patches)
	}
	for _, want := range []string{corev1alpha1.ApprovedRevisionAnnotation, corev1alpha1.ApproveDigestAnnotation, "digest-reviewed"} {
		if !strings.Contains(patches[0], want) {
			t.Fatalf("patch %s does not carry %s", patches[0], want)
		}
	}
	for _, recorded := range []string{corev1alpha1.ApprovedByAnnotation, corev1alpha1.ApprovedAtAnnotation, corev1alpha1.ApprovedDigestAnnotation} {
		if strings.Contains(patches[0], recorded) {
			t.Fatalf("CLI patch sets %s, which only the admission webhook may record", recorded)
		}
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
