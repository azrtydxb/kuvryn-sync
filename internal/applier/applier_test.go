package applier

import (
	"context"
	"testing"

	corev1alpha1 "github.com/azrtydxb/solder/api/v1alpha1"
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
