package retry

import (
	"fmt"
	"time"

	corev1alpha1 "github.com/azrtydxb/solder/api/v1alpha1"
)

// State tracks attempts for one desired revision.
type State struct {
	DesiredRevision string
	Attempts        int32
	LastFailureAt   time.Time
}

// Decision records whether another attempt may start.
type Decision struct {
	Allowed bool
	Reason  string
}

// Decide prevents retry loops for a failed desired revision.
func Decide(policy corev1alpha1.FailurePolicy, state State, now time.Time, backoff time.Duration) Decision {
	maxAttempts := int32(1)
	if policy.MaxAttempts != nil && *policy.MaxAttempts > 0 {
		maxAttempts = *policy.MaxAttempts
	}
	if state.Attempts >= maxAttempts {
		return Decision{Reason: fmt.Sprintf("maxAttempts %d reached for desired revision %s", maxAttempts, state.DesiredRevision)}
	}
	if backoff > 0 && !state.LastFailureAt.IsZero() && now.Before(state.LastFailureAt.Add(backoff)) {
		return Decision{Reason: "backoff window has not elapsed"}
	}
	return Decision{Allowed: true}
}
