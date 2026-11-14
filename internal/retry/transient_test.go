package retry

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	corev1alpha1 "github.com/azrtydxb/kuvryn-sync/api/v1alpha1"
)

func statusError(code int32, reason metav1.StatusReason, message string) error {
	return &apierrors.StatusError{ErrStatus: metav1.Status{Status: metav1.StatusFailure, Code: code, Reason: reason, Message: message}}
}

// Catches a classifier that counts API-server and webhook outages as
// failures to spend maxAttempts on, and one that retries denials forever.
func TestIsTransient(t *testing.T) {
	gr := schema.GroupResource{Group: "apps", Resource: "deployments"}
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"webhook EOF", apierrors.NewInternalError(errors.New(`failed calling webhook "webhook.cert-manager.io": failed to call webhook: Post "https://cert-manager-webhook.cert-manager.svc:443/validate?timeout=30s": EOF`)), true},
		{"webhook connection refused", statusError(500, metav1.StatusReasonUnknown, `failed calling webhook "vcluster.cnpg.io": failed to call webhook: Post "https://x.svc:443/v": dial tcp 10.0.0.1:443: connect: connection refused`), true},
		{"webhook connection reset", statusError(500, metav1.StatusReasonUnknown, `failed calling webhook "x": read tcp: connection reset by peer`), true},
		{"webhook i/o timeout", statusError(500, metav1.StatusReasonUnknown, `failed calling webhook "x": dial tcp 10.0.0.1:443: i/o timeout`), true},
		{"webhook deadline", statusError(500, metav1.StatusReasonUnknown, `failed calling webhook "x": context deadline exceeded`), true},
		{"webhook no endpoints", statusError(500, metav1.StatusReasonUnknown, `failed calling webhook "x": no endpoints available for service "y"`), true},
		{"internal error", apierrors.NewInternalError(errors.New("boom")), true},
		{"service unavailable", apierrors.NewServiceUnavailable("down"), true},
		{"server timeout", apierrors.NewServerTimeout(gr, "get", 1), true},
		{"timeout", apierrors.NewTimeoutError("slow", 1), true},
		{"too many requests", apierrors.NewTooManyRequests("slow down", 1), true},
		{"net error", &net.OpError{Op: "dial", Err: errors.New("unreachable")}, true},
		{"wrapped transient", fmt.Errorf("apply: %w", apierrors.NewServiceUnavailable("down")), true},
		{"webhook denial", statusError(400, metav1.StatusReasonBadRequest, `admission webhook "validate.x.io" denied the request: replicas must be positive`), false},
		{"denial mentioning webhook", statusError(422, metav1.StatusReasonInvalid, `admission webhook "validate.x.io" denied the request: failed calling webhook EOF in field`), false},
		{"invalid", apierrors.NewInvalid(schema.GroupKind{Kind: "Deployment"}, "x", nil), false},
		{"forbidden", apierrors.NewForbidden(gr, "x", errors.New("no")), false},
		{"bad request", apierrors.NewBadRequest("bad"), false},
		{"conflict", apierrors.NewConflict(gr, "x", errors.New("changed")), false},
		{"not found", apierrors.NewNotFound(gr, "x"), false},
		{"plain error", errors.New("render failed"), false},
		{"context canceled", context.Canceled, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsTransient(tc.err); got != tc.want {
				t.Fatalf("IsTransient(%v) = %t; want %t", tc.err, got, tc.want)
			}
		})
	}
}

// Catches a transient failure that still stops at maxAttempts, and a backoff
// that is skipped for one.
func TestDecideTransientIgnoresMaxAttempts(t *testing.T) {
	max := int32(3)
	policy := corev1alpha1.FailurePolicy{MaxAttempts: &max}
	now := time.Unix(100000, 0)
	state := State{DesiredRevision: "abc", Attempts: 3, LastFailureAt: now.Add(-time.Hour), Transient: true}
	if d := Decide(policy, state, now, time.Minute); !d.Allowed {
		t.Fatalf("decision = %+v; a transient failure must not exhaust maxAttempts", d)
	}
	state.LastFailureAt = now
	if d := Decide(policy, state, now, time.Minute); d.Allowed || d.Exhausted {
		t.Fatalf("decision = %+v; want a backoff wait that is not exhausted", d)
	}
	state.Transient = false
	state.LastFailureAt = now.Add(-time.Hour)
	if d := Decide(policy, state, now, time.Minute); !d.Exhausted {
		t.Fatalf("decision = %+v; a non-transient failure still exhausts maxAttempts", d)
	}
}

func TestTransientBackoff(t *testing.T) {
	if got := TransientBackoff(0); got != time.Second {
		t.Fatalf("backoff(0) = %v", got)
	}
	if got := TransientBackoff(3); got != 8*time.Second {
		t.Fatalf("backoff(3) = %v", got)
	}
	if got := TransientBackoff(50); got != 5*time.Minute {
		t.Fatalf("backoff(50) = %v; want the 5 minute cap", got)
	}
}
