package resource

import (
	"testing"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func object(apiVersion, kind string) unstructured.Unstructured {
	obj := unstructured.Unstructured{}
	obj.SetAPIVersion(apiVersion)
	obj.SetKind(kind)
	obj.SetName("x")
	return obj
}

func crd(group, kind, scope string) unstructured.Unstructured {
	obj := object("apiextensions.k8s.io/v1", "CustomResourceDefinition")
	obj.Object["spec"] = map[string]any{"group": group, "scope": scope, "names": map[string]any{"kind": kind}}
	return obj
}

func TestScopesResolveMappedAndRenderedKinds(t *testing.T) {
	mapper := meta.NewDefaultRESTMapper(nil)
	mapper.Add(schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "storage.k8s.io", Version: "v1", Kind: "StorageClass"}, meta.RESTScopeRoot)
	scopes := NewScopes(mapper, []unstructured.Unstructured{
		crd("cert-manager.io", "ClusterIssuer", "Cluster"),
		crd("cert-manager.io", "Certificate", "Namespaced"),
	})

	cases := []struct {
		obj  unstructured.Unstructured
		want bool
	}{
		{object("v1", "ConfigMap"), true},
		{object("storage.k8s.io/v1", "StorageClass"), false},
		{object("cert-manager.io/v1", "ClusterIssuer"), false},
		{object("cert-manager.io/v1", "Certificate"), true},
	}
	for _, tc := range cases {
		got, err := scopes.Namespaced(tc.obj)
		if err != nil {
			t.Fatalf("%s: %v", tc.obj.GetKind(), err)
		}
		if got != tc.want {
			t.Fatalf("%s: namespaced = %v, want %v", tc.obj.GetKind(), got, tc.want)
		}
	}
}

func TestScopesRejectUnknownKinds(t *testing.T) {
	scopes := NewScopes(meta.NewDefaultRESTMapper(nil), nil)
	if _, err := scopes.Namespaced(object("example.com/v1", "Widget")); err == nil {
		t.Fatal("unknown kind resolved without error")
	}
}
