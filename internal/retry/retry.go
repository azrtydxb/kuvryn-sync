package retry

import (
	"fmt"
	"time"

	corev1alpha1 "github.com/azrtydxb/solder/api/v1alpha1"
)

// State tracks attempts for one desired revision.
type State struct {
	DesiredRevision  string
	DeployedRevision string
	Attempts         int32
	LastFailureAt    time.Time
	Suspended        bool
}

// Decision records whether another attempt may start.
type Decision struct {
	Allowed          bool
	Reason           string
	DesiredRevision  string
	DeployedRevision string
	NextAttemptAfter time.Time
}

// Decide prevents retry loops for a failed desired revision.
func Decide(policy corev1alpha1.FailurePolicy, state State, now time.Time, backoff time.Duration) Decision {
	decision := Decision{Allowed: true, DesiredRevision: state.DesiredRevision, DeployedRevision: state.DeployedRevision}
	if state.Suspended {
		decision.Allowed = false
		decision.Reason = "Application is suspended"
		return decision
	}
	maxAttempts := int32(1)
	if policy.MaxAttempts != nil && *policy.MaxAttempts > 0 {
		maxAttempts = *policy.MaxAttempts
	}
	if state.Attempts >= maxAttempts {
		decision.Allowed = false
		decision.Reason = fmt.Sprintf("maxAttempts %d reached for desired revision %s", maxAttempts, state.DesiredRevision)
		return decision
	}
	if backoff > 0 && !state.LastFailureAt.IsZero() {
		next := state.LastFailureAt.Add(backoff)
		if now.Before(next) {
			decision.Allowed = false
			decision.Reason = "backoff window has not elapsed"
			decision.NextAttemptAfter = next
			return decision
		}
	}
	return decision
}

// ReportHonestRevisions copies desired/deployed revision truth into Application status.
func ReportHonestRevisions(app *corev1alpha1.Application, desired, deployed string) {
	app.Status.DesiredRevision = desired
	app.Status.DeployedRevision = deployed
}
