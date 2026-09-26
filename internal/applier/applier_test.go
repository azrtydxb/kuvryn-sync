package applier

import (
	"context"
	"errors"
	"testing"

	corev1alpha1 "github.com/azrtydxb/kuvryn-sync/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func TestApplyIsIdempotentAndMarksOwnership(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	obj := configMap("settings", "one")
	a := Applier{Client: c, ApplicationNamespace: "default"}
	for i := 0; i < 2; i++ {
		if err := a.Apply(context.Background(), "payments", "abc123", []unstructured.Unstructured{obj}, corev1alpha1.ConflictPolicyFail); err != nil {
			t.Fatalf("apply %d: %v", i, err)
		}
	}
	var got unstructured.Unstructured
	got.SetAPIVersion("v1")
	got.SetKind("ConfigMap")
	if err := c.Get(context.Background(), client.ObjectKey{Namespace: "payments", Name: "settings"}, &got); err != nil {
		t.Fatal(err)
	}
	if got.GetLabels()[ApplicationLabelKey] != "payments" || got.GetLabels()[ApplicationNamespaceLabelKey] != "default" || got.GetAnnotations()[RevisionAnnotationKey] != "abc123" {
		t.Fatalf("missing ownership metadata labels=%v annotations=%v", got.GetLabels(), got.GetAnnotations())
	}
}

func TestApplyRejectsUnsupportedConflictPolicy(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	a := Applier{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	err := a.Apply(context.Background(), "payments", "abc123", []unstructured.Unstructured{configMap("settings", "one")}, corev1alpha1.ConflictPolicy("force"))
	if err == nil {
		t.Fatal("expected unsupported conflict policy error")
	}
}

func TestApplyForcesOwnershipOnlyWhenAdopting(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	for policy, wantForce := range map[corev1alpha1.ConflictPolicy]bool{corev1alpha1.ConflictPolicyFail: false, corev1alpha1.ConflictPolicyAdopt: true} {
		var got client.ApplyOptions
		c := interceptor.NewClient(fake.NewClientBuilder().WithScheme(scheme).Build(), interceptor.Funcs{
			Apply: func(ctx context.Context, c client.WithWatch, obj runtime.ApplyConfiguration, opts ...client.ApplyOption) error {
				got.ApplyOptions(opts)
				return c.Apply(ctx, obj, opts...)
			},
		})
		if err := (Applier{Client: c}).Apply(context.Background(), "payments", "abc123", []unstructured.Unstructured{configMap("settings", "one")}, policy); err != nil {
			t.Fatalf("%s: %v", policy, err)
		}
		if got.FieldManager != FieldManager || (got.Force != nil && *got.Force) != wantForce {
			t.Fatalf("%s: field manager %q, force %v", policy, got.FieldManager, got.Force)
		}
	}
}

func configMap(name, value string) unstructured.Unstructured {
	return unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]any{
			"name":      name,
			"namespace": "payments",
		},
		"data": map[string]any{"value": value},
	}}
}

func TestApplyUsesTheKuvrynSyncFieldManager(t *testing.T) {
	if FieldManager != "kuvryn-sync" {
		t.Fatalf("FieldManager = %q, want kuvryn-sync", FieldManager)
	}
}

// TestOnlyLegacyConflictsAreTakenOver proves apply forces ownership only
// when every conflict the API server names is with the legacy
// before-first-apply owner. proved by: returning true for any conflict makes
// the mixed and foreign cases fail.
func TestOnlyLegacyConflictsAreTakenOver(t *testing.T) {
	conflict := func(managers ...string) error {
		causes := make([]metav1.StatusCause, 0, len(managers))
		for _, m := range managers {
			causes = append(causes, metav1.StatusCause{Type: metav1.CauseTypeFieldManagerConflict, Message: `conflict with "` + m + `" using v1`, Field: ".data.key"})
		}
		return &apierrors.StatusError{ErrStatus: metav1.Status{Status: metav1.StatusFailure, Code: 409, Reason: metav1.StatusReasonConflict, Details: &metav1.StatusDetails{Causes: causes}}}
	}
	for _, tc := range []struct {
		name string
		err  error
		want bool
	}{
		{"legacy only", conflict(LegacyFieldManager), true},
		{"legacy and another manager", conflict(LegacyFieldManager, "argocd-application-controller"), false},
		{"another manager", conflict("helm"), false},
		{"a conflict without causes", conflict(), false},
		{"not a conflict", errors.New("boom"), false},
	} {
		if got := onlyLegacyConflicts(tc.err); got != tc.want {
			t.Errorf("%s: onlyLegacyConflicts = %v, want %v", tc.name, got, tc.want)
		}
	}
}
