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
	"slices"

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	corev1alpha1 "github.com/azrtydxb/solder/api/v1alpha1"
	"github.com/azrtydxb/solder/internal/health"
	"github.com/azrtydxb/solder/internal/resource"
)

// RolloutCompleteCondition is the Revision condition that tracks its rollout:
// False from the first apply until every group is Healthy, then True. Unlike
// the phase, which planning, approval, dependency waits and failures all
// overwrite, it survives every path until the rollout finishes.
const RolloutCompleteCondition = "RolloutComplete"

type rolloutState int

const (
	rolloutNotStarted rolloutState = iota
	rolloutInProgress
	rolloutComplete
)

// rolloutProgress is what applyAndObserve needs to know about the rollout it
// continues or starts.
type rolloutProgress struct {
	// previousPhase is the phase persisted before this reconcile.
	previousPhase corev1alpha1.RevisionPhase
	state         rolloutState
	// hooks maps the hooks this rollout already applied to their state.
	hooks map[corev1alpha1.ResourceRef]corev1alpha1.HealthState
}

// rolloutOf reads how far a Revision's rollout got. Revisions written before
// the condition existed fall back to their persisted phase.
func rolloutOf(revision *corev1alpha1.Revision, previousPhase corev1alpha1.RevisionPhase) rolloutState {
	if condition := apimeta.FindStatusCondition(revision.Status.Conditions, RolloutCompleteCondition); condition != nil {
		if condition.Status == metav1.ConditionTrue {
			return rolloutComplete
		}
		return rolloutInProgress
	}
	switch previousPhase {
	case corev1alpha1.RevisionPhaseHealthy, corev1alpha1.RevisionPhaseRolledBack:
		return rolloutComplete
	case corev1alpha1.RevisionPhaseApplying, corev1alpha1.RevisionPhaseObserving:
		return rolloutInProgress
	default:
		return rolloutNotStarted
	}
}

func setRolloutComplete(revision *corev1alpha1.Revision, complete bool) {
	condition := metav1.Condition{
		Type: RolloutCompleteCondition, Status: metav1.ConditionFalse, Reason: "RollingOut",
		Message: "Rollout started and not every group is Healthy yet", ObservedGeneration: revision.Generation,
	}
	if complete {
		condition.Status, condition.Reason, condition.Message = metav1.ConditionTrue, "Complete", "Every group of the rollout became Healthy"
	}
	apimeta.SetStatusCondition(&revision.Status.Conditions, condition)
}

// finishedPhase is the phase a completed rollout rests in.
func finishedPhase(previousPhase corev1alpha1.RevisionPhase) corev1alpha1.RevisionPhase {
	if previousPhase == corev1alpha1.RevisionPhaseRolledBack {
		return previousPhase
	}
	return corev1alpha1.RevisionPhaseHealthy
}

func resourceRef(id resource.ID) corev1alpha1.ResourceRef {
	return corev1alpha1.ResourceRef{APIVersion: id.APIVersion(), Kind: id.Kind, Namespace: id.Namespace, Name: id.Name}
}

func objectRef(obj unstructured.Unstructured) (corev1alpha1.ResourceRef, bool) {
	id, err := resource.FromObject(obj)
	if err != nil {
		return corev1alpha1.ResourceRef{}, false
	}
	return resourceRef(id), true
}

// hookStates maps each hook this Revision's rollout applied to its last
// recorded state.
func hookStates(hooks []corev1alpha1.HookStatus) map[corev1alpha1.ResourceRef]corev1alpha1.HealthState {
	out := make(map[corev1alpha1.ResourceRef]corev1alpha1.HealthState, len(hooks))
	for _, hook := range hooks {
		out[hook.Resource] = hook.State
	}
	return out
}

// withoutCompletedHooks drops hooks that already succeeded for this Revision.
// They ran once; a Job cleaned up after it finished is neither drift nor a
// reason to run it again.
func withoutCompletedHooks(objects []unstructured.Unstructured, states map[corev1alpha1.ResourceRef]corev1alpha1.HealthState) []unstructured.Unstructured {
	return slices.DeleteFunc(slices.Clone(objects), func(obj unstructured.Unstructured) bool {
		ref, ok := objectRef(obj)
		return ok && states[ref] == corev1alpha1.HealthStateHealthy
	})
}

// splitHooks sorts a hook group by what this rollout already did with each
// hook: pending hooks are applied, watched ones were applied and are only
// checked, and done ones succeeded and count as Healthy without a look.
func splitHooks(objects []unstructured.Unstructured, states map[corev1alpha1.ResourceRef]corev1alpha1.HealthState) (pending, watched []unstructured.Unstructured, done []health.Result) {
	for _, obj := range objects {
		id, err := resource.FromObject(obj)
		if err != nil {
			pending = append(pending, obj)
			continue
		}
		switch state, recorded := states[resourceRef(id)]; {
		case !recorded:
			pending = append(pending, obj)
		case state == corev1alpha1.HealthStateHealthy:
			done = append(done, health.Result{Resource: id, State: state, Reason: "HookSucceeded", Message: "hook already succeeded for this Revision"})
		default:
			watched = append(watched, obj)
		}
	}
	return pending, watched, done
}

// recordHooks updates the Revision's hook statuses with fresh results,
// keeping hooks recorded earlier in the rollout.
func recordHooks(revision *corev1alpha1.Revision, stage string, results []health.Result) {
	for _, result := range results {
		ref := resourceRef(result.Resource)
		status := corev1alpha1.HookStatus{Resource: ref, Stage: stage, State: result.State, Message: result.Message}
		if i := slices.IndexFunc(revision.Status.Hooks, func(h corev1alpha1.HookStatus) bool { return h.Resource == ref }); i >= 0 {
			revision.Status.Hooks[i] = status
			continue
		}
		revision.Status.Hooks = append(revision.Status.Hooks, status)
	}
}
