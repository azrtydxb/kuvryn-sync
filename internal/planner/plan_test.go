package planner

import (
	"testing"

	corev1alpha1 "github.com/azrtydxb/solder/api/v1alpha1"
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

func TestDeleteChangesAreDestructiveAndWarnForHighRiskPrune(t *testing.T) {
	obj := secret("db", map[string]any{"password": "old"})
	obj.SetAnnotations(map[string]string{"solder.io/prune": "disabled"})
	plan, err := Build(nil, []unstructured.Unstructured{obj})
	if err != nil {
		t.Fatal(err)
	}
	res := plan.RevisionPlan(10).Resources[0]
	if !res.Destructive {
		t.Fatalf("delete not marked destructive: %#v", res)
	}
	if len(res.Warnings) < 3 {
		t.Fatalf("expected destructive, opt-out, and high-risk warnings: %#v", res.Warnings)
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
