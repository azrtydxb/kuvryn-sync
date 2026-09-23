package ops

import (
	"context"
	"regexp"

	corev1alpha1 "github.com/azrtydxb/solder/api/v1alpha1"
)

var boundedLabel = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,63}$`)

// MetricLabels returns bounded low-cardinality metric labels.
func MetricLabels(app corev1alpha1.Application, phase corev1alpha1.RevisionPhase) map[string]string {
	return map[string]string{
		"namespace": safeLabel(app.Namespace, "default"),
		"sync":      safeLabel(string(app.Status.Sync.State), "unknown"),
		"health":    safeLabel(string(app.Status.Health.State), "unknown"),
		"phase":     safeLabel(string(phase), "unknown"),
	}
}

// Tracer is an optional tracing seam; nil means tracing disabled.
type Tracer interface {
	Start(context.Context, string) (context.Context, func(error))
}

func safeLabel(value, fallback string) string {
	if !boundedLabel.MatchString(value) {
		return fallback
	}
	return value
}
