package planner

import (
	"testing"

	corev1alpha1 "github.com/azrtydxb/kuvryn-sync/api/v1alpha1"
	"github.com/azrtydxb/kuvryn-sync/internal/prune"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestBuildClassifiesCreateUpdateDeleteUnchanged(t *testing.T) {
	desired := []unstructured.Unstructured{
		cm("create", "new"),
		cm("update", "desired"),
		cm("same", "stable"),
	}
	live := []unstructured.Unstructured{
		cm("update", "live"),
		cm("delete", "old"),
		withServerFields(cm("same", "stable")),
	}
	plan, err := Build(desired, live)
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}
	if plan.Summary.Create != 1 || plan.Summary.Update != 1 || plan.Summary.Delete != 1 || plan.Summary.Unchanged != 1 {
		t.Fatalf("summary = %#v", plan.Summary)
	}
	got := map[string]corev1alpha1.PlanAction{}
	for _, change := range plan.Changes {
		got[change.ID.Name] = change.Action
	}
	want := map[string]corev1alpha1.PlanAction{"create": corev1alpha1.PlanActionCreate, "update": corev1alpha1.PlanActionUpdate, "delete": corev1alpha1.PlanActionDelete, "same": corev1alpha1.PlanActionUnchanged}
	for name, action := range want {
		if got[name] != action {
			t.Fatalf("action[%s] = %s, want %s", name, got[name], action)
		}
	}
}

func TestRevisionPlanIsBounded(t *testing.T) {
	plan, err := Build([]unstructured.Unstructured{cm("a", "1"), cm("b", "2")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	revisionPlan := plan.RevisionPlan(1)
	if len(revisionPlan.Resources) != 1 {
		t.Fatalf("resources = %d, want 1", len(revisionPlan.Resources))
	}
	if !revisionPlan.Summary.Truncated {
		t.Fatal("expected truncated summary")
	}
}

func TestSecretChangesAreRedacted(t *testing.T) {
	desired := secret("db", map[string]any{"password": "new"})
	live := secret("db", map[string]any{"password": "old", "token": "old"})
	plan, err := Build([]unstructured.Unstructured{desired}, []unstructured.Unstructured{live})
	if err != nil {
		t.Fatal(err)
	}
	got := plan.RevisionPlan(10).Resources[0].Changes
	if len(got) != 1 {
		t.Fatalf("changes = %#v", got)
	}
	if !got[0].Redacted || got[0].Before != "2 key(s) REDACTED" || got[0].After != "1 key(s) REDACTED" {
		t.Fatalf("secret change not redacted: %#v", got[0])
	}
}

func TestSensitiveNonSecretFieldsAreRedacted(t *testing.T) {
	desired := cm("settings", "unused")
	live := cm("settings", "unused")
	desired.Object["data"] = map[string]any{"password": "new-secret"}
	live.Object["data"] = map[string]any{"password": "old-secret"}
	plan, err := Build([]unstructured.Unstructured{desired}, []unstructured.Unstructured{live})
	if err != nil {
		t.Fatal(err)
	}
	changes := plan.RevisionPlan(10).Resources[0].Changes
	if len(changes) != 1 || !changes[0].Redacted || changes[0].Before != "REDACTED" || changes[0].After != "REDACTED" {
		t.Fatalf("sensitive field not redacted: %#v", changes)
	}
}

func TestDeleteChangesAreDestructiveAndWarn(t *testing.T) {
	obj := secret("db", map[string]any{"password": "old"})
	plan, err := Build(nil, []unstructured.Unstructured{obj})
	if err != nil {
		t.Fatal(err)
	}
	res := plan.RevisionPlan(10).Resources[0]
	if !res.Destructive {
		t.Fatalf("delete not marked destructive: %#v", res)
	}
	if len(res.Warnings) != 1 || res.Warnings[0] != "delete action is destructive" {
		t.Fatalf("expected the destructive warning: %#v", res.Warnings)
	}
}

func TestKeepListsKeptObjectsInOrder(t *testing.T) {
	plan, err := Build([]unstructured.Unstructured{cm("b", "v")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Keep([]prune.Rejected{{Object: cm("c", "v"), Reason: "opted out"}, {Object: cm("a", "v"), Reason: "high-risk"}}); err != nil {
		t.Fatal(err)
	}
	resources := plan.RevisionPlan(10).Resources
	names := []string{resources[0].Resource.Name, resources[1].Resource.Name, resources[2].Resource.Name}
	if names[0] != "a" || names[1] != "b" || names[2] != "c" {
		t.Fatalf("plan is not sorted: %v", names)
	}
	if resources[0].Action != "Unchanged" || resources[0].Warnings[0] != "no longer in desired state; prune skipped: high-risk" {
		t.Fatalf("kept object = %#v", resources[0])
	}
	if plan.Summary.Unchanged != 2 || plan.Summary.Create != 1 {
		t.Fatalf("summary = %#v", plan.Summary)
	}
}

func TestSSAConflictsAreDetectedWithFailPolicy(t *testing.T) {
	desired := cm("owned", "desired")
	live := cm("owned", "live")
	live.SetManagedFields([]metav1.ManagedFieldsEntry{{
		Manager:    "kubectl",
		Operation:  metav1.ManagedFieldsOperationApply,
		APIVersion: "v1",
		FieldsType: "FieldsV1",
		FieldsV1:   &metav1.FieldsV1{Raw: []byte(`{"f:data":{"f:value":{}}}`)},
	}})
	plan, err := Build([]unstructured.Unstructured{desired}, []unstructured.Unstructured{live})
	if err != nil {
		t.Fatal(err)
	}
	conflicts := plan.RevisionPlan(10).Resources[0].Conflicts
	if len(conflicts) == 0 {
		t.Fatalf("expected conflict in %#v", plan.RevisionPlan(10).Resources[0])
	}
	if conflicts[0].Manager != "kubectl" || conflicts[0].Policy != corev1alpha1.ConflictPolicyFail {
		t.Fatalf("unexpected conflict: %#v", conflicts[0])
	}
}

func cm(name, value string) unstructured.Unstructured {
	obj := unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]any{
			"name":      name,
			"namespace": "payments",
		},
		"data": map[string]any{"value": value},
	}}
	return obj
}

func secret(name string, data map[string]any) unstructured.Unstructured {
	obj := unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Secret",
		"metadata": map[string]any{
			"name":      name,
			"namespace": "payments",
		},
		"data": data,
	}}
	return obj
}

func withServerFields(obj unstructured.Unstructured) unstructured.Unstructured {
	obj.SetResourceVersion("123")
	obj.SetUID("abc")
	obj.Object["status"] = map[string]any{"observed": true}
	return obj
}

func managedBy(manager, fields string) metav1.ManagedFieldsEntry {
	return metav1.ManagedFieldsEntry{Manager: manager, Operation: metav1.ManagedFieldsOperationApply, APIVersion: "v1", FieldsType: "FieldsV1", FieldsV1: &metav1.FieldsV1{Raw: []byte(fields)}}
}

func TestFieldsOtherManagersOrTheServerOwnAreNotChanges(t *testing.T) {
	desired := cm("shared", "same")
	live := cm("shared", "same")
	live.SetLabels(map[string]string{"kustomize.toolkit.fluxcd.io/name": "payments"})
	_ = unstructured.SetNestedField(live.Object, "defaulted", "data", "serverDefault")
	live.SetManagedFields([]metav1.ManagedFieldsEntry{
		managedBy("kuvryn-sync", `{"f:data":{"f:value":{}}}`),
		managedBy("kustomize-controller", `{"f:metadata":{"f:labels":{"f:kustomize.toolkit.fluxcd.io/name":{}}}}`),
	})
	plan, err := Build([]unstructured.Unstructured{desired}, []unstructured.Unstructured{live})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Summary.Unchanged != 1 || plan.Summary.Update != 0 {
		t.Fatalf("foreign or defaulted fields were planned as changes: %#v", plan.RevisionPlan(10))
	}
}

func TestRemovingAFieldKuvrynSyncOwnedIsAChange(t *testing.T) {
	desired := cm("shrinking", "same")
	live := cm("shrinking", "same")
	_ = unstructured.SetNestedField(live.Object, "old", "data", "removed")
	live.SetManagedFields([]metav1.ManagedFieldsEntry{managedBy("kuvryn-sync", `{"f:data":{"f:value":{},"f:removed":{}}}`)})
	plan, err := Build([]unstructured.Unstructured{desired}, []unstructured.Unstructured{live})
	if err != nil {
		t.Fatal(err)
	}
	fields := plan.RevisionPlan(10).Resources[0].Changes
	if plan.Summary.Update != 1 || len(fields) != 1 || fields[0].Path != "data.removed" || fields[0].After != "" {
		t.Fatalf("plan = %#v", plan.RevisionPlan(10))
	}
}

func TestConflictsAreReportedOnlyForExactlyOwnedFields(t *testing.T) {
	desired := cm("mixed", "desired")
	desired.SetLabels(map[string]string{"team": "payments"})
	live := cm("mixed", "live")
	live.SetLabels(map[string]string{"team": "search"})
	live.SetManagedFields([]metav1.ManagedFieldsEntry{
		managedBy("kustomize-controller", `{"f:metadata":{"f:labels":{"f:team":{}}}}`),
		managedBy("kuvryn-sync", `{"f:data":{"f:value":{}}}`),
	})
	plan, err := Build([]unstructured.Unstructured{desired}, []unstructured.Unstructured{live})
	if err != nil {
		t.Fatal(err)
	}
	conflicts := plan.RevisionPlan(10).Resources[0].Conflicts
	if len(conflicts) != 1 || conflicts[0].Path != "metadata.labels.team" || conflicts[0].Manager != "kustomize-controller" {
		t.Fatalf("conflicts = %#v", conflicts)
	}
}

func TestAFieldKuvrynSyncSharesWithAnotherManagerConflicts(t *testing.T) {
	// Both managers applied the same value, so both own data.value; kuvryn-sync is
	// listed last so a single-owner map would lose the other manager.
	live := cm("shared", "live")
	live.SetManagedFields([]metav1.ManagedFieldsEntry{
		managedBy("kubectl", `{"f:data":{"f:value":{}}}`),
		managedBy("kuvryn-sync", `{"f:data":{"f:value":{}}}`),
	})
	plan, err := Build([]unstructured.Unstructured{cm("shared", "desired")}, []unstructured.Unstructured{live})
	if err != nil {
		t.Fatal(err)
	}
	conflicts := plan.RevisionPlan(10).Resources[0].Conflicts
	if len(conflicts) != 1 || conflicts[0].Path != "data.value" || conflicts[0].Manager != "kubectl" {
		t.Fatalf("conflicts = %#v, want one on data.value with kubectl", conflicts)
	}

	// A field Kuvryn Sync shares is still Kuvryn Sync's: dropping it is a change.
	removed := cm("shared", "live")
	_ = unstructured.SetNestedField(removed.Object, "old", "data", "removed")
	removed.SetManagedFields([]metav1.ManagedFieldsEntry{
		managedBy("kuvryn-sync", `{"f:data":{"f:value":{},"f:removed":{}}}`),
		managedBy("kubectl", `{"f:data":{"f:removed":{}}}`),
	})
	plan, err = Build([]unstructured.Unstructured{cm("shared", "live")}, []unstructured.Unstructured{removed})
	if err != nil {
		t.Fatal(err)
	}
	fields := plan.RevisionPlan(10).Resources[0].Changes
	if len(fields) != 1 || fields[0].Path != "data.removed" {
		t.Fatalf("changes = %#v, want the removal of data.removed", fields)
	}
}

func TestListItemsAreMatchedByKeyNotOwnedWholesale(t *testing.T) {
	deployment := func(image string, extra map[string]any) unstructured.Unstructured {
		container := map[string]any{"name": "api", "image": image}
		for k, v := range extra {
			container[k] = v
		}
		return unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "apps/v1", "kind": "Deployment",
			"metadata": map[string]any{"name": "api", "namespace": "payments"},
			"spec":     map[string]any{"template": map[string]any{"spec": map[string]any{"containers": []any{container}}}},
		}}
	}
	live := deployment("nginx:1", map[string]any{"imagePullPolicy": "Always", "terminationMessagePath": "/dev/termination-log"})
	live.SetManagedFields([]metav1.ManagedFieldsEntry{managedBy("kuvryn-sync",
		`{"f:spec":{"f:template":{"f:spec":{"f:containers":{"k:{\"name\":\"api\"}":{".":{},"f:image":{},"f:name":{}}}}}}}`)})

	unchanged, err := Build([]unstructured.Unstructured{deployment("nginx:1", nil)}, []unstructured.Unstructured{live})
	if err != nil {
		t.Fatal(err)
	}
	if unchanged.Summary.Unchanged != 1 {
		t.Fatalf("server-defaulted container fields were planned as changes: %#v", unchanged.RevisionPlan(10))
	}

	changed, err := Build([]unstructured.Unstructured{deployment("nginx:2", nil)}, []unstructured.Unstructured{live})
	if err != nil {
		t.Fatal(err)
	}
	fields := changed.RevisionPlan(10).Resources[0].Changes
	if len(fields) != 1 || fields[0].Path != "spec.template.spec.containers[0].image" || fields[0].After != "nginx:2" {
		t.Fatalf("image change = %#v", fields)
	}
}
