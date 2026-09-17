package prune

import (
	"testing"

	"github.com/azrtydxb/solder/internal/applier"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestPlanOnlyPrunesManagedEligibleResources(t *testing.T) {
	eligible := cm("eligible")
	eligible.SetLabels(map[string]string{applier.ApplicationLabelKey: "payments"})
	unmanaged := cm("unmanaged")
	optout := cm("optout")
	optout.SetLabels(map[string]string{applier.ApplicationLabelKey: "payments"})
	optout.SetAnnotations(map[string]string{PruneAnnotationKey: "disabled"})
	secret := obj("v1", "Secret", "payments", "db")
	secret.SetLabels(map[string]string{applier.ApplicationLabelKey: "payments"})
	result := Plan([]unstructured.Unstructured{unmanaged, optout, eligible, secret}, Policy{Application: "payments"})
	if len(result.Eligible) != 1 || result.Eligible[0].GetName() != "eligible" {
		t.Fatalf("eligible = %#v", names(result.Eligible))
	}
	if len(result.Rejected) != 3 {
		t.Fatalf("rejected = %#v", result.Rejected)
	}
}

func TestPlanAllowsHighRiskOnlyWhenPolicyAllowsIt(t *testing.T) {
	secret := obj("v1", "Secret", "payments", "db")
	secret.SetLabels(map[string]string{applier.ApplicationLabelKey: "payments"})
	if got := Plan([]unstructured.Unstructured{secret}, Policy{Application: "payments"}); len(got.Eligible) != 0 {
		t.Fatalf("secret should be rejected without high-risk approval")
	}
	if got := Plan([]unstructured.Unstructured{secret}, Policy{Application: "payments", AllowHighRisk: true}); len(got.Eligible) != 1 {
		t.Fatalf("secret should be eligible with high-risk approval")
	}
}

func cm(name string) unstructured.Unstructured {
	return obj("v1", "ConfigMap", "payments", name)
}

func obj(apiVersion, kind, namespace, name string) unstructured.Unstructured {
	return unstructured.Unstructured{Object: map[string]any{"apiVersion": apiVersion, "kind": kind, "metadata": map[string]any{"namespace": namespace, "name": name}}}
}

func names(objects []unstructured.Unstructured) []string {
	out := make([]string, len(objects))
	for i, obj := range objects {
		out[i] = obj.GetName()
	}
	return out
}
