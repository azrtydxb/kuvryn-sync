package health

import (
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	corev1alpha1 "github.com/azrtydxb/solder/api/v1alpha1"
)

func rolloutCheck(rules ...corev1alpha1.HealthRule) corev1alpha1.HealthCheck {
	return corev1alpha1.HealthCheck{
		ObjectMeta: metav1.ObjectMeta{Name: "rollouts"},
		Spec:       corev1alpha1.HealthCheckSpec{Group: "argoproj.io", Kind: "Rollout", Rules: rules},
	}
}

func rollout(phase string) unstructured.Unstructured {
	o := obj("argoproj.io/v1alpha1", "Rollout", "payments", "api")
	_ = unstructured.SetNestedField(o.Object, phase, "status", "phase")
	return o
}

func TestHealthCheckRulesDecideMatchingKinds(t *testing.T) {
	evaluator, err := NewEvaluator([]corev1alpha1.HealthCheck{rolloutCheck(
		corev1alpha1.HealthRule{Expression: `object.status.phase == "Degraded"`, State: corev1alpha1.HealthStateDegraded, Message: "rollout degraded"},
		corev1alpha1.HealthRule{Expression: `object.status.phase == "Healthy"`, State: corev1alpha1.HealthStateHealthy},
	)})
	if err != nil {
		t.Fatal(err)
	}

	degraded, err := evaluator.Evaluate(rollout("Degraded"))
	if err != nil {
		t.Fatal(err)
	}
	if degraded.State != corev1alpha1.HealthStateDegraded || degraded.Reason != "HealthCheck" || degraded.Message != "rollout degraded" {
		t.Fatalf("degraded rollout = %#v", degraded)
	}

	// No rule matches: kstatus decides, and a status without conditions is Healthy.
	fallback, err := evaluator.Evaluate(rollout("Paused"))
	if err != nil {
		t.Fatal(err)
	}
	if fallback.State != corev1alpha1.HealthStateHealthy || fallback.Reason != "Ready" {
		t.Fatalf("unmatched rollout = %#v", fallback)
	}

	// Other kinds never see the rules.
	other, err := evaluator.Evaluate(deployment("api", 3, 1))
	if err != nil {
		t.Fatal(err)
	}
	if other.Reason != "ReplicasUnavailable" {
		t.Fatalf("deployment = %#v", other)
	}
}

func TestHealthCheckCompileErrorsAreReported(t *testing.T) {
	for expression, want := range map[string]string{
		`object.status.phase ==`:   "Syntax error",
		`object.status.phase + ""`: "must evaluate to a bool",
		`"not a bool"`:             "must evaluate to a bool",
	} {
		if _, err := CompileRule(expression); err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("CompileRule(%q) error = %v, want %q", expression, err, want)
		}
	}
	evaluator, err := NewEvaluator([]corev1alpha1.HealthCheck{rolloutCheck(
		corev1alpha1.HealthRule{Expression: `object.status.phase ==`, State: corev1alpha1.HealthStateHealthy},
	)})
	if err == nil || !strings.Contains(err.Error(), "HealthCheck rollouts rule 0") {
		t.Fatalf("NewEvaluator error = %v", err)
	}
	if got, _ := evaluator.Evaluate(rollout("Degraded")); got.Reason == "HealthCheck" {
		t.Fatal("invalid rule was applied")
	}
}

func TestHealthCheckOverCostLimitFailsClosed(t *testing.T) {
	evaluator, err := NewEvaluator([]corev1alpha1.HealthCheck{rolloutCheck(corev1alpha1.HealthRule{
		Expression: `object.spec.items.all(x, object.spec.items.all(y, x + y >= 0))`,
		State:      corev1alpha1.HealthStateHealthy,
	})})
	if err != nil {
		t.Fatal(err)
	}
	expensive := rollout("Healthy")
	items := make([]any, 1000)
	for i := range items {
		items[i] = int64(i)
	}
	_ = unstructured.SetNestedSlice(expensive.Object, items, "spec", "items")

	got, err := evaluator.Evaluate(expensive)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != corev1alpha1.HealthStateProgressing || got.Reason != "HealthCheckFailed" || !strings.Contains(got.Message, "cost") {
		t.Fatalf("expensive rule = %#v", got)
	}
}
