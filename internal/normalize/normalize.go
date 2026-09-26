package normalize

import (
	"encoding/json"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Object removes expected server-populated fields before diffing. For
// built-in API types it also removes fields set to null: those types decode
// into Go structs, so the API server stores null and absent alike as absent.
// Custom resources keep their nulls, because a CRD field marked nullable
// preserves an explicit null.
func Object(obj unstructured.Unstructured) (unstructured.Unstructured, error) {
	copy := obj.DeepCopy()
	if builtinGroup(copy.GroupVersionKind().Group) {
		dropNulls(copy.Object)
	}
	unstructured.RemoveNestedField(copy.Object, "status")
	metadata, ok, err := unstructured.NestedMap(copy.Object, "metadata")
	if err != nil {
		return unstructured.Unstructured{}, err
	}
	if ok {
		for _, field := range []string{"creationTimestamp", "deletionGracePeriodSeconds", "deletionTimestamp", "generation", "managedFields", "resourceVersion", "selfLink", "uid"} {
			delete(metadata, field)
		}
		annotations, ok := metadata["annotations"].(map[string]any)
		if ok {
			for _, key := range []string{"kubectl.kubernetes.io/last-applied-configuration"} {
				delete(annotations, key)
			}
			if len(annotations) == 0 {
				delete(metadata, "annotations")
			} else {
				metadata["annotations"] = annotations
			}
		}
		copy.Object["metadata"] = metadata
	}
	return *copy, nil
}

// Equal reports semantic equality after JSON canonicalization.
func Equal(a, b unstructured.Unstructured) (bool, error) {
	an, err := Object(a)
	if err != nil {
		return false, err
	}
	bn, err := Object(b)
	if err != nil {
		return false, err
	}
	aj, err := json.Marshal(an.Object)
	if err != nil {
		return false, err
	}
	bj, err := json.Marshal(bn.Object)
	if err != nil {
		return false, err
	}
	return string(aj) == string(bj), nil
}

// builtinGroups are the API groups Kubernetes itself serves. Some CRDs use
// *.k8s.io groups too (Gateway API, volume snapshots), so this is a list, not
// a suffix rule.
var builtinGroups = map[string]bool{
	"": true, "apps": true, "batch": true, "autoscaling": true, "policy": true,
	"admissionregistration.k8s.io": true, "apiextensions.k8s.io": true, "apiregistration.k8s.io": true,
	"authentication.k8s.io": true, "authorization.k8s.io": true, "certificates.k8s.io": true,
	"coordination.k8s.io": true, "discovery.k8s.io": true, "events.k8s.io": true,
	"flowcontrol.apiserver.k8s.io": true, "internal.apiserver.k8s.io": true, "networking.k8s.io": true,
	"node.k8s.io": true, "rbac.authorization.k8s.io": true, "resource.k8s.io": true,
	"scheduling.k8s.io": true, "storage.k8s.io": true, "storagemigration.k8s.io": true,
}

// builtinGroup reports whether group is served by Kubernetes itself.
func builtinGroup(group string) bool { return builtinGroups[group] }

// dropNulls removes map entries whose value is null, at every depth, including
// inside maps held in lists. List items that are themselves null are kept,
// since removing them would shift the indexes of the others.
func dropNulls(v any) {
	switch typed := v.(type) {
	case map[string]any:
		for k, item := range typed {
			if item == nil {
				delete(typed, k)
				continue
			}
			dropNulls(item)
		}
	case []any:
		for _, item := range typed {
			dropNulls(item)
		}
	}
}
