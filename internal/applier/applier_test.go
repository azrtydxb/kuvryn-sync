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
		result, err := a.Apply(context.Background(), "payments", "abc123", []unstructured.Unstructured{obj}, "")
		if err != nil {
			t.Fatalf("apply %d: %v", i, err)
		}
		if result.Applied != 1 {
			t.Fatalf("applied = %d", result.Applied)
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
	_, err := a.Apply(context.Background(), "payments", "abc123", []unstructured.Unstructured{configMap("settings", "one")}, corev1alpha1.ConflictPolicy("force"))
	if err == nil {
		t.Fatal("expected unsupported conflict policy error")
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
