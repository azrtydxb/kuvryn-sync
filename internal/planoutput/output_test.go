package planoutput

import (
	"bytes"
	"strings"
	"testing"

	corev1alpha1 "github.com/azrtydxb/solder/api/v1alpha1"
)

func TestWriteTextRedactsSecretsAndMarksDeletes(t *testing.T) {
	doc := Document{Application: "payments", Revision: "abc123", Plan: corev1alpha1.RevisionPlan{
		Summary: corev1alpha1.PlanSummary{Create: 1, Update: 1, Delete: 1, Unchanged: 2},
		Resources: []corev1alpha1.PlanResourceChange{
			{Resource: corev1alpha1.ResourceRef{APIVersion: "v1", Kind: "Secret", Namespace: "payments", Name: "db"}, Action: corev1alpha1.PlanActionUpdate, Changes: []corev1alpha1.PlanFieldChange{{Path: "data.password", Before: "old-password", After: "new-password"}}},
			{Resource: corev1alpha1.ResourceRef{APIVersion: "v1", Kind: "ConfigMap", Namespace: "payments", Name: "legacy"}, Action: corev1alpha1.PlanActionDelete, Destructive: true, Warnings: []string{"delete action is destructive"}},
		},
	}}
	var out bytes.Buffer
	if err := Write(&out, doc, "text"); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	for _, forbidden := range []string{"old-password", "new-password"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("output leaked %q: %s", forbidden, text)
		}
	}
	for _, required := range []string{"Application: payments", "Revision:    abc123", "REDACTED", "! DELETE ConfigMap/payments/legacy", "1 deleted"} {
		if !strings.Contains(text, required) {
			t.Fatalf("output missing %q: %s", required, text)
		}
	}
}

func TestWriteJSONIsMachineReadableAndRedacted(t *testing.T) {
	doc := Document{Plan: corev1alpha1.RevisionPlan{Resources: []corev1alpha1.PlanResourceChange{{Resource: corev1alpha1.ResourceRef{Kind: "Secret", Name: "db"}, Action: corev1alpha1.PlanActionUpdate, Changes: []corev1alpha1.PlanFieldChange{{Path: "stringData.token", Before: "old", After: "new"}}}}}}
	var out bytes.Buffer
	if err := Write(&out, doc, "json"); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	if strings.Contains(text, "old") || strings.Contains(text, "new") {
		t.Fatalf("json leaked secret value: %s", text)
	}
	if !strings.Contains(text, "\"redacted\": true") {
		t.Fatalf("json missing redaction flag: %s", text)
	}
}
