package normalize

import (
	"encoding/json"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Object removes expected server-populated fields before diffing.
func Object(obj unstructured.Unstructured) (unstructured.Unstructured, error) {
	copy := obj.DeepCopy()
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
