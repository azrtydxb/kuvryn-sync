package ops

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"time"

	corev1alpha1 "github.com/azrtydxb/solder/api/v1alpha1"
)

var boundedLabel = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,63}$`)

// LifecycleEvent is a bounded, redacted lifecycle event/log shape.
type LifecycleEvent struct {
	Application string                     `json:"application"`
	Revision    string                     `json:"revision,omitempty"`
	Phase       corev1alpha1.RevisionPhase `json:"phase"`
	Reason      string                     `json:"reason"`
	Message     string                     `json:"message"`
}

// NewLifecycleEvent creates a structured lifecycle event without secret fields.
func NewLifecycleEvent(app, revision string, phase corev1alpha1.RevisionPhase, reason, message string) LifecycleEvent {
	return LifecycleEvent{Application: truncate(app, 63), Revision: truncate(revision, 64), Phase: phase, Reason: truncate(reason, 63), Message: truncate(message, 256)}
}

// MetricLabels returns bounded low-cardinality metric labels.
func MetricLabels(app corev1alpha1.Application, phase corev1alpha1.RevisionPhase) map[string]string {
	return map[string]string{
		"namespace": safeLabel(app.Namespace, "default"),
		"sync":      safeLabel(string(app.Status.Sync.State), "unknown"),
		"health":    safeLabel(string(app.Status.Health.State), "unknown"),
		"phase":     safeLabel(string(phase), "unknown"),
	}
}

// Tracer is an optional tracing seam; nil/noop means tracing disabled.
type Tracer interface {
	Start(context.Context, string) (context.Context, func(error))
}

type noopTracer struct{}

// NoopTracer returns a disabled tracer.
func NoopTracer() Tracer { return noopTracer{} }

func (noopTracer) Start(ctx context.Context, _ string) (context.Context, func(error)) {
	return ctx, func(error) {}
}

// RateLimiter provides deterministic per-key backoff suitable for reconcile storms.
type RateLimiter struct {
	Base time.Duration
	Max  time.Duration
}

func (r RateLimiter) Delay(failures int) time.Duration {
	base := r.Base
	if base <= 0 {
		base = time.Second
	}
	max := r.Max
	if max <= 0 {
		max = time.Minute
	}
	d := base
	for i := 0; i < failures; i++ {
		d *= 2
		if d >= max {
			return max
		}
	}
	return d
}

// ScaleFixture summarizes deterministic scale-test inputs.
type ScaleFixture struct {
	Applications int `json:"applications"`
	Resources    int `json:"resources"`
	Revisions    int `json:"revisions"`
}

func (s ScaleFixture) Validate() error {
	if s.Applications <= 0 || s.Resources < s.Applications || s.Revisions < s.Applications {
		return fmt.Errorf("invalid scale fixture: applications/resources/revisions must be positive and resources/revisions >= applications")
	}
	return nil
}

// HALeaderElectionDefault records the production HA default wired in cmd/main.go.
func HALeaderElectionDefault() string { return "e3d625f0.solder.io" }

func safeLabel(value, fallback string) string {
	if !boundedLabel.MatchString(value) {
		return fallback
	}
	return value
}

func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit]
}

// StableLabelKeys returns labels in sorted order for tests and exporters.
func StableLabelKeys(labels map[string]string) []string {
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
