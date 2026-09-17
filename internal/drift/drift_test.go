package drift

import (
	"testing"

	corev1alpha1 "github.com/azrtydxb/solder/api/v1alpha1"
	"github.com/azrtydxb/solder/internal/applier"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestEnqueueOwnerMapsManagedResourceToApplication(t *testing.T) {
	obj := cm("settings", "one")
	obj.SetLabels(map[string]string{applier.ApplicationLabelKey: "payments", applier.ApplicationNamespaceLabelKey: "default"})
	request, ok := EnqueueOwner(obj)
	if !ok || request.Namespace != "default" || request.Name != "payments" {
		t.Fatalf("request=%#v ok=%v", request, ok)
	}
	if _, ok := EnqueueOwner(cm("unmanaged", "one")); ok {
		t.Fatal("unmanaged object enqueued")
	}
}

func TestClassifyIgnoresServerMutationAndDetectsRealDrift(t *testing.T) {
	desired := cm("settings", "one")
	live := cm("settings", "one")
	live.SetResourceVersion("123")
	live.Object["status"] = map[string]any{"ignored": true}
	result, err := Classify([]unstructured.Unstructured{desired}, []unstructured.Unstructured{live})
	if err != nil {
		t.Fatal(err)
	}
	if result.State != corev1alpha1.SyncStateSynced {
		t.Fatalf("server mutation caused drift: %#v", result)
	}
	live = cm("settings", "two")
	result, err = Classify([]unstructured.Unstructured{desired}, []unstructured.Unstructured{live})
	if err != nil {
		t.Fatal(err)
	}
	if result.State != corev1alpha1.SyncStateDrifted || result.Plan.Summary.Update != 1 {
		t.Fatalf("real drift not detected: %#v", result)
	}
}

func TestApplyStatusKeepsHealthIndependent(t *testing.T) {
	app := &corev1alpha1.Application{}
	ApplyStatus(app, Result{State: corev1alpha1.SyncStateDrifted})
	if app.Status.Sync.State != corev1alpha1.SyncStateDrifted {
		t.Fatalf("sync = %s", app.Status.Sync.State)
	}
	if app.Status.Health.State != corev1alpha1.HealthStateUnknown {
		t.Fatalf("health changed unexpectedly: %s", app.Status.Health.State)
	}
}

func TestShouldSelfHealRequiresEnablementAndSafety(t *testing.T) {
	app := corev1alpha1.Application{ObjectMeta: metav1.ObjectMeta{Name: "payments"}}
	result := Result{State: corev1alpha1.SyncStateDrifted}
	if err := ShouldSelfHeal(app, result); err == nil {
		t.Fatal("disabled self-heal allowed")
	}
	app.Spec.Sync.SelfHeal = true
	if err := ShouldSelfHeal(app, result); err != nil {
		t.Fatalf("enabled self-heal rejected: %v", err)
	}
	app.Spec.Suspend = true
	if err := ShouldSelfHeal(app, result); err == nil {
		t.Fatal("suspended self-heal allowed")
	}
}

func cm(name, value string) unstructured.Unstructured {
	return unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]any{
			"name":      name,
			"namespace": "payments",
		},
		"data": map[string]any{"value": value},
	}}
}
