package drift

import (
	"fmt"

	corev1alpha1 "github.com/azrtydxb/solder/api/v1alpha1"
	"github.com/azrtydxb/solder/internal/applier"
	"github.com/azrtydxb/solder/internal/planner"
	"github.com/azrtydxb/solder/internal/syncpolicy"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// WatchRequest is the Application reconcile target derived from a managed resource event.
type WatchRequest struct {
	Namespace string
	Name      string
}

// EnqueueOwner maps a Solder-managed object back to its owning Application.
func EnqueueOwner(obj unstructured.Unstructured) (WatchRequest, bool) {
	labels := obj.GetLabels()
	name := labels[applier.ApplicationLabelKey]
	if name == "" {
		return WatchRequest{}, false
	}
	namespace := labels[applier.ApplicationNamespaceLabelKey]
	if namespace == "" {
		namespace = obj.GetNamespace()
	}
	return WatchRequest{Namespace: namespace, Name: name}, true
}

// Result summarizes desired/live drift independently of health.
type Result struct {
	State corev1alpha1.SyncState
	Plan  corev1alpha1.RevisionPlan
}

// Classify compares desired and live resources after normal Kubernetes normalization.
func Classify(desired, live []unstructured.Unstructured) (Result, error) {
	plan, err := planner.Build(desired, live)
	if err != nil {
		return Result{}, err
	}
	state := corev1alpha1.SyncStateSynced
	if plan.Summary.Create+plan.Summary.Update+plan.Summary.Delete > 0 {
		state = corev1alpha1.SyncStateDrifted
	}
	return Result{State: state, Plan: plan.RevisionPlan(50)}, nil
}

// ApplyStatus records drift in Application sync state without changing health state.
func ApplyStatus(app *corev1alpha1.Application, result Result) {
	app.Status.Sync.State = result.State
	if app.Status.Health.State == "" {
		app.Status.Health.State = corev1alpha1.HealthStateUnknown
	}
}

// ShouldSelfHeal decides whether Solder may reapply desired state for drift.
func ShouldSelfHeal(app corev1alpha1.Application, result Result) error {
	if result.State != corev1alpha1.SyncStateDrifted {
		return fmt.Errorf("self-heal is only needed for Drifted state")
	}
	if !app.Spec.Sync.SelfHeal {
		return fmt.Errorf("self-heal is disabled")
	}
	if err := syncpolicy.EnsureMutationAllowed(app, corev1alpha1.RevisionPhaseApplying); err != nil {
		return err
	}
	for _, res := range result.Plan.Resources {
		if len(res.Conflicts) > 0 {
			return fmt.Errorf("self-heal blocked by SSA conflict on %s/%s", res.Resource.Kind, res.Resource.Name)
		}
	}
	return nil
}
