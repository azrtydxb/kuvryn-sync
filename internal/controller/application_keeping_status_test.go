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
	"encoding/json"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1alpha1 "github.com/azrtydxb/kuvryn-sync/api/v1alpha1"
)

var _ = Describe("Controller writes keep the parent's spec ownership", func() {
	const appName = "keep-owner-app"
	ctx := context.Background()
	key := types.NamespacedName{Name: appName, Namespace: "default"}

	// applyAsParent writes the Application the way the parent Application
	// does: server-side apply under the kuvryn-sync field manager, with the
	// non-canonical 10m duration strings the Git repo ships. The controller
	// round-trips those through metav1.Duration as 10m0s, so a full-object
	// write would make the API server hand the two timeout fields to the
	// writer and the parent's next apply (conflictPolicy: fail) would
	// conflict.
	applyAsParent := func() {
		app := &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "sync.kuvryn.io/v1alpha1",
			"kind":       "Application",
			"metadata":   map[string]any{"name": appName, "namespace": "default"},
			"spec": map[string]any{
				"serviceAccountName": "kuvryn-sync-deployer",
				"source": map[string]any{
					"repositoryRef": map[string]any{"name": "platform"},
					"path":          "apps/payments",
					"render":        map[string]any{"type": "yaml"},
				},
				"destination": map[string]any{"namespace": "payments"},
				"sync":        map[string]any{"automatic": true, "conflictPolicy": "fail", "prune": true, "selfHeal": true},
				"strategy": map[string]any{
					"type":          "rolling",
					"failurePolicy": map[string]any{"action": "rollback", "maxAttempts": int64(3), "timeout": "10m"},
				},
				"health":         map[string]any{"timeout": "10m"},
				"deletionPolicy": "Orphan",
			},
		}}
		Expect(k8sClient.Apply(ctx, client.ApplyConfigurationFromUnstructured(app), client.FieldOwner("kuvryn-sync"))).To(Succeed())
	}

	// ownedFieldPaths flattens the leaf fields a manager and operation holds,
	// as "spec/health/timeout"-style paths.
	ownedFieldPaths := func(live client.Object, manager string, operation metav1.ManagedFieldsOperationType) map[string]bool {
		paths := map[string]bool{}
		for _, mf := range live.GetManagedFields() {
			if mf.Manager != manager || mf.Operation != operation || mf.FieldsV1 == nil {
				continue
			}
			var root map[string]any
			Expect(json.Unmarshal(mf.FieldsV1.Raw, &root)).To(Succeed())
			var walk func(map[string]any, string)
			walk = func(node map[string]any, prefix string) {
				for k, v := range node {
					key := strings.TrimPrefix(k, "f:")
					path := key
					if prefix != "" {
						path = prefix + "/" + key
					}
					if child, ok := v.(map[string]any); ok && len(child) > 0 {
						walk(child, path)
					} else {
						paths[path] = true
					}
				}
			}
			walk(root, "")
		}
		return paths
	}

	AfterEach(func() {
		deleteObject(ctx, &corev1alpha1.Application{ObjectMeta: metav1.ObjectMeta{Name: appName, Namespace: "default"}})
	})

	It("does not claim spec fields it only round-trips", func() {
		applyAsParent()

		app := &corev1alpha1.Application{}
		Expect(k8sClient.Get(ctx, key, app)).To(Succeed())
		// The controller's annotation-only write, e.g. recording a rollback.
		setRollbackRequest(app, rollbackRequest{target: "abc123", from: "def456", kind: corev1alpha1.RollbackKindManual})
		reconciler := newApplicationReconciler(nil, nil)
		Expect(reconciler.updateKeepingStatus(ctx, app)).To(Succeed())

		live := &unstructured.Unstructured{}
		live.SetAPIVersion("sync.kuvryn.io/v1alpha1")
		live.SetKind("Application")
		Expect(k8sClient.Get(ctx, key, live)).To(Succeed())

		parent := ownedFieldPaths(live, "kuvryn-sync", metav1.ManagedFieldsOperationApply)
		Expect(parent).To(HaveKey("spec/health/timeout"), "kuvryn-sync must still own spec.health.timeout")
		Expect(parent).To(HaveKey("spec/strategy/failurePolicy/timeout"), "kuvryn-sync must still own spec.strategy.failurePolicy.timeout")

		fresh := &corev1alpha1.Application{}
		Expect(k8sClient.Get(ctx, key, fresh)).To(Succeed())
		Expect(fresh.Annotations).To(HaveKeyWithValue(corev1alpha1.RollbackRevisionAnnotation, "abc123"), "the rollback bookkeeping was persisted")
	})
})
