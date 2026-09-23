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

package controller

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/gomega"
)

// ensureCustomKind installs a namespaced, schemaless test CRD such as
// widgets.example.com and waits until it is served.
func ensureCustomKind(ctx context.Context, kind, plural string) {
	crd := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apiextensions.k8s.io/v1",
		"kind":       "CustomResourceDefinition",
		"metadata":   map[string]any{"name": plural + ".example.com"},
		"spec": map[string]any{
			"group": "example.com",
			"scope": "Namespaced",
			"names": map[string]any{"kind": kind, "plural": plural, "singular": plural[:len(plural)-1], "listKind": kind + "List"},
			"versions": []any{map[string]any{
				"name": "v1", "served": true, "storage": true,
				"schema": map[string]any{"openAPIV3Schema": map[string]any{"type": "object", "x-kubernetes-preserve-unknown-fields": true}},
			}},
		},
	}}
	if err := k8sClient.Create(ctx, crd); client.IgnoreAlreadyExists(err) != nil {
		Expect(err).NotTo(HaveOccurred())
	}
	Eventually(func(g Gomega) {
		list := &unstructured.UnstructuredList{}
		list.SetGroupVersionKind(schema.GroupVersionKind{Group: "example.com", Version: "v1", Kind: kind + "List"})
		g.Expect(k8sClient.List(ctx, list)).To(Succeed())
	}, 30*time.Second, 200*time.Millisecond).Should(Succeed())
}

func customObject(kind, name, value string) unstructured.Unstructured {
	return unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "example.com/v1",
		"kind":       kind,
		"metadata":   map[string]any{"name": name},
		"spec":       map[string]any{"value": value},
	}}
}
