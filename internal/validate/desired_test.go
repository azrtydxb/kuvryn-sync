package validate

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestDesiredAcceptsUniqueResources(t *testing.T) {
	objects := []unstructured.Unstructured{
		object("v1", "ConfigMap", "payments", "runtime"),
		object("apps/v1", "Deployment", "payments", "api"),
	}
	if err := Desired(objects, Options{DestinationNamespace: "payments"}); err != nil {
		t.Fatalf("validate desired: %v", err)
	}
}

func TestDesiredRejectsDuplicateIdentities(t *testing.T) {
	objects := []unstructured.Unstructured{
		object("v1", "ConfigMap", "payments", "runtime"),
		object("v1", "ConfigMap", "payments", "runtime"),
	}
	if err := Desired(objects, Options{DestinationNamespace: "payments"}); err == nil {
		t.Fatal("expected duplicate identity error")
	}
}

func TestDesiredRejectsMissingMetadata(t *testing.T) {
	objects := []unstructured.Unstructured{{Object: map[string]any{"apiVersion": "v1", "kind": "ConfigMap"}}}
	if err := Desired(objects, Options{}); err == nil {
		t.Fatal("expected missing metadata.name error")
	}
}

func TestDesiredRejectsDestinationNamespaceViolation(t *testing.T) {
	objects := []unstructured.Unstructured{object("v1", "ConfigMap", "other", "runtime")}
	err := Desired(objects, Options{DestinationNamespace: "payments"})
	if err == nil {
		t.Fatal("expected namespace violation")
	}
	if !strings.Contains(err.Error(), "other/runtime") {
		t.Fatalf("violation does not name the object: %v", err)
	}
}

func object(apiVersion, kind, namespace, name string) unstructured.Unstructured {
	obj := unstructured.Unstructured{Object: map[string]any{"apiVersion": apiVersion, "kind": kind}}
	obj.SetNamespace(namespace)
	obj.SetName(name)
	return obj
}
