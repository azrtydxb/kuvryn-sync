package graph

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestBuildInfersCommonRelationshipsDeterministically(t *testing.T) {
	deploy := obj("apps/v1", "Deployment", "payments", "api")
	deploy.SetLabels(map[string]string{"app": "api"})
	_ = unstructured.SetNestedSlice(deploy.Object, []any{map[string]any{"name": "data", "persistentVolumeClaim": map[string]any{"claimName": "data"}}}, "spec", "template", "spec", "volumes")
	_ = unstructured.SetNestedSlice(deploy.Object, []any{map[string]any{"name": "api", "envFrom": []any{map[string]any{"configMapRef": map[string]any{"name": "settings"}}}}}, "spec", "template", "spec", "containers")
	service := obj("v1", "Service", "payments", "api")
	_ = unstructured.SetNestedStringMap(service.Object, map[string]string{"app": "api"}, "spec", "selector")
	pvc := obj("v1", "PersistentVolumeClaim", "payments", "data")
	cm := obj("v1", "ConfigMap", "payments", "settings")
	child := obj("v1", "ConfigMap", "payments", "owned")
	child.SetOwnerReferences([]metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "Deployment", Name: "api"}})
	g, err := Build([]unstructured.Unstructured{deploy, service, pvc, cm, child})
	if err != nil {
		t.Fatal(err)
	}
	want := map[EdgeType]bool{EdgeSelects: false, EdgeMountsPVC: false, EdgeDependsOn: false, EdgeOwns: false}
	for _, edge := range g.Edges {
		if _, ok := want[edge.Type]; ok {
			want[edge.Type] = true
		}
	}
	for typ, seen := range want {
		if !seen {
			t.Fatalf("missing edge type %s in %#v", typ, g.Edges)
		}
	}
	if len(g.Children(g.Edges[0].From)) == 0 {
		t.Fatalf("expected graph children")
	}
}

func obj(apiVersion, kind, namespace, name string) unstructured.Unstructured {
	return unstructured.Unstructured{Object: map[string]any{"apiVersion": apiVersion, "kind": kind, "metadata": map[string]any{"namespace": namespace, "name": name}}}
}
