package resource

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Scopes decides whether rendered objects are namespaced.
type Scopes struct {
	mapper   meta.RESTMapper
	rendered map[schema.GroupKind]bool
}

// NewScopes resolves scope through the cluster's RESTMapper and, for custom
// resources whose CustomResourceDefinition is rendered alongside them and not
// yet installed, through that definition's spec.scope.
func NewScopes(mapper meta.RESTMapper, rendered []unstructured.Unstructured) Scopes {
	scopes := Scopes{mapper: mapper, rendered: map[schema.GroupKind]bool{}}
	for _, obj := range rendered {
		if obj.GetKind() != "CustomResourceDefinition" {
			continue
		}
		group, _, _ := unstructured.NestedString(obj.Object, "spec", "group")
		kind, _, _ := unstructured.NestedString(obj.Object, "spec", "names", "kind")
		scope, _, _ := unstructured.NestedString(obj.Object, "spec", "scope")
		if kind != "" {
			scopes.rendered[schema.GroupKind{Group: group, Kind: kind}] = scope != "Cluster"
		}
	}
	return scopes
}

// Namespaced reports whether obj's kind is namespaced.
func (s Scopes) Namespaced(obj unstructured.Unstructured) (bool, error) {
	gvk := obj.GroupVersionKind()
	mapping, err := s.mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err == nil {
		return mapping.Scope.Name() == meta.RESTScopeNameNamespace, nil
	}
	if namespaced, ok := s.rendered[gvk.GroupKind()]; ok && meta.IsNoMatchError(err) {
		return namespaced, nil
	}
	return false, fmt.Errorf("resolve scope of %s: %w", gvk.String(), err)
}
