package resource

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ID uniquely identifies a Kubernetes object.
type ID struct {
	Group     string
	Version   string
	Kind      string
	Namespace string
	Name      string
}

// FromObject returns the stable identity for obj.
func FromObject(obj unstructured.Unstructured) (ID, error) {
	gv, err := schema.ParseGroupVersion(obj.GetAPIVersion())
	if err != nil {
		return ID{}, fmt.Errorf("parse apiVersion: %w", err)
	}
	if obj.GetKind() == "" || obj.GetName() == "" {
		return ID{}, fmt.Errorf("kind and name are required")
	}
	return ID{Group: gv.Group, Version: gv.Version, Kind: obj.GetKind(), Namespace: obj.GetNamespace(), Name: obj.GetName()}, nil
}

func (id ID) APIVersion() string {
	return schema.GroupVersion{Group: id.Group, Version: id.Version}.String()
}

func (id ID) String() string {
	if id.Namespace == "" {
		return fmt.Sprintf("%s/%s/%s", id.APIVersion(), id.Kind, id.Name)
	}
	return fmt.Sprintf("%s/%s/%s/%s", id.APIVersion(), id.Kind, id.Namespace, id.Name)
}
