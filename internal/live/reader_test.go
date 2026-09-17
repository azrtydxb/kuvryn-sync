package live

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestReaderLoadsFoundAndMissingDesiredIdentities(t *testing.T) {
	scheme := runtime.NewScheme()
	found := cm("found")
	missing := cm("missing")
	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(&found).Build()
	result, err := (Reader{Client: client}).Read(context.Background(), []unstructured.Unstructured{found, missing})
	if err != nil {
		t.Fatalf("read live: %v", err)
	}
	if len(result.Found) != 1 {
		t.Fatalf("found = %d, want 1", len(result.Found))
	}
	if len(result.Missing) != 1 || result.Missing[0].Name != "missing" {
		t.Fatalf("missing = %#v", result.Missing)
	}
}

func cm(name string) unstructured.Unstructured {
	return unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]any{
			"name":      name,
			"namespace": "payments",
		},
	}}
}
