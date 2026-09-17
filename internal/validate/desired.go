/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package validate

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Options configures desired-state validation.
type Options struct {
	DestinationNamespace string
}

// ResourceID uniquely identifies a rendered Kubernetes object.
type ResourceID struct {
	APIVersion string
	Kind       string
	Namespace  string
	Name       string
}

// Desired validates rendered objects before any mutation is planned.
func Desired(objects []unstructured.Unstructured, options Options) error {
	seen := map[ResourceID]struct{}{}
	for i := range objects {
		obj := &objects[i]
		id, err := identify(obj, options)
		if err != nil {
			return err
		}
		if _, ok := seen[id]; ok {
			return fmt.Errorf("duplicate rendered resource %s/%s %s/%s", id.APIVersion, id.Kind, id.Namespace, id.Name)
		}
		seen[id] = struct{}{}
	}
	return nil
}

func identify(obj *unstructured.Unstructured, options Options) (ResourceID, error) {
	apiVersion := strings.TrimSpace(obj.GetAPIVersion())
	kind := strings.TrimSpace(obj.GetKind())
	name := strings.TrimSpace(obj.GetName())
	namespace := strings.TrimSpace(obj.GetNamespace())
	if apiVersion == "" {
		return ResourceID{}, fmt.Errorf("rendered resource is missing apiVersion")
	}
	if kind == "" {
		return ResourceID{}, fmt.Errorf("rendered resource %s is missing kind", apiVersion)
	}
	if name == "" {
		return ResourceID{}, fmt.Errorf("rendered resource %s/%s is missing metadata.name", apiVersion, kind)
	}
	if options.DestinationNamespace != "" && namespace != "" && namespace != options.DestinationNamespace {
		return ResourceID{}, fmt.Errorf("rendered resource %s/%s %s/%s targets namespace outside destination %s", apiVersion, kind, namespace, name, options.DestinationNamespace)
	}
	if namespace == "" {
		namespace = options.DestinationNamespace
	}
	return ResourceID{APIVersion: apiVersion, Kind: kind, Namespace: namespace, Name: name}, nil
}
