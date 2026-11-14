package retry

import (
	"errors"
	"net"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// MaxTransientBackoff caps the wait between retries of a transient failure.
const MaxTransientBackoff = 5 * time.Minute

// TransientBackoff is how long to wait before retrying after failures
// transient failures in a row: exponentially from one second, capped at
// MaxTransientBackoff.
func TransientBackoff(failures int32) time.Duration {
	if failures < 0 {
		failures = 0
	}
	return min(time.Second<<min(failures, 20), MaxTransientBackoff)
}

// webhookOutage are the causes that make a failed webhook call an outage of
// the webhook rather than its verdict on the object.
var webhookOutage = []string{
	"eof",
	"connection refused",
	"connection reset",
	"i/o timeout",
	"context deadline exceeded",
	"no endpoints available",
	"tls handshake timeout",
	"client rate limiter",
}

// IsTransient reports whether err is a failure of the API server, an
// admission webhook or the network that is likely to pass by itself, so
// retrying the same desired state can help without counting against
// maxAttempts. A verdict on the object (invalid, forbidden, bad request,
// conflict, not found, or a webhook that denied it) is never transient.
func IsTransient(err error) bool {
	if err == nil {
		return false
	}
	switch {
	case apierrors.IsInvalid(err), apierrors.IsForbidden(err), apierrors.IsBadRequest(err),
		apierrors.IsConflict(err), apierrors.IsNotFound(err), apierrors.IsAlreadyExists(err),
		apierrors.IsUnauthorized(err), apierrors.IsMethodNotSupported(err), apierrors.IsGone(err):
		return false
	case apierrors.IsInternalError(err), apierrors.IsServiceUnavailable(err), apierrors.IsServerTimeout(err),
		apierrors.IsTimeout(err), apierrors.IsTooManyRequests(err):
		return true
	}
	if message := strings.ToLower(err.Error()); strings.Contains(message, "failed calling webhook") {
		for _, cause := range webhookOutage {
			if strings.Contains(message, cause) {
				return true
			}
		}
		return false
	}
	var netErr net.Error
	return errors.As(err, &netErr)
}
