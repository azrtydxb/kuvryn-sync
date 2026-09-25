package console

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	authenticationv1 "k8s.io/api/authentication/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	authenticationclient "k8s.io/client-go/kubernetes/typed/authentication/v1"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/azrtydxb/kuvryn-sync/internal/redact"
)

const (
	// maxTokenBytes is the largest token accepted; anything larger is refused
	// before a request is made.
	maxTokenBytes = 16 << 10
	// maxTokenForm bounds the sign-in form body: a 16 KiB token, form-encoded,
	// with room to spare.
	maxTokenForm = 64 << 10
	// maxTokenSession is the longest a token session lasts, whatever the
	// token's own expiry.
	maxTokenSession = 8 * time.Hour
	// reviewTimeout bounds the SelfSubjectReview that validates a token.
	reviewTimeout = 10 * time.Second
	// reviewPath is the only request the sign-in client may send.
	reviewPath = "/apis/authentication.k8s.io/v1/selfsubjectreviews"
)

var (
	// errTokenRefused marks a token that is not a usable credential: it
	// yields /login?error=token.
	errTokenRefused = errors.New("console: token refused")
	// errClusterUnavailable marks an API server that could not judge the
	// token: it yields /login?error=cluster.
	errClusterUnavailable = errors.New("console: the API server could not validate the token")
)

// reviewedUser is who the API server says a token belongs to.
type reviewedUser struct {
	Username string
	Groups   []string
}

// normalizeToken trims surrounding whitespace and a "Bearer " prefix, and
// refuses an empty token, one over 16 KiB, and one with any character a
// bearer token cannot hold.
func normalizeToken(raw string) (string, error) {
	token := strings.TrimSpace(raw)
	if len(token) > len("Bearer ") && strings.EqualFold(token[:len("Bearer ")], "Bearer ") {
		token = strings.TrimSpace(token[len("Bearer "):])
	}
	switch {
	case token == "":
		return "", fmt.Errorf("%w: empty", errTokenRefused)
	case len(token) > maxTokenBytes:
		return "", fmt.Errorf("%w: larger than %d bytes", errTokenRefused, maxTokenBytes)
	}
	for i := 0; i < len(token); i++ {
		if c := token[i]; c <= ' ' || c >= 0x7f {
			return "", fmt.Errorf("%w: holds a character a bearer token cannot", errTokenRefused)
		}
	}
	return token, nil
}

// tokenExpiry returns when a session for token ends: at the JWT's exp claim
// when the token is a JWT that carries one, and never later than 8 hours
// after now. It refuses a JWT whose exp has passed. The claim is read, not
// verified; the API server verifies the token itself.
func tokenExpiry(token string, now time.Time) (time.Time, error) {
	limit := now.Add(maxTokenSession)
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return limit, nil
	}
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	if err != nil {
		return limit, nil
	}
	var claims struct {
		Exp *json.Number `json:"exp"`
	}
	if json.Unmarshal(payload, &claims) != nil || claims.Exp == nil {
		return limit, nil
	}
	seconds, err := claims.Exp.Float64()
	if err != nil || seconds >= float64(limit.Unix()) {
		// Unreadable, or later than the cap: the cap applies. This also
		// keeps values too large for an int64 away from the conversion.
		return limit, nil
	}
	exp := time.Unix(int64(seconds), 0)
	if !now.Before(exp) {
		return time.Time{}, fmt.Errorf("%w: expired at %s", errTokenRefused, exp.UTC().Format(time.RFC3339))
	}
	if exp.Before(limit) {
		return exp, nil
	}
	return limit, nil
}

// selfSubjectReview asks the API server who token belongs to, with a
// SelfSubjectReview sent with that token. The client is built from
// tokenConfig, so it carries none of the console's credentials, and its
// transport sends nothing but that one POST.
func (a *Auth) selfSubjectReview(ctx context.Context, token string) (reviewedUser, error) {
	cfg := tokenConfig(a.base, token)
	cfg.Timeout = reviewTimeout
	origin, err := apiOrigin(cfg)
	if err != nil {
		return reviewedUser{}, fmt.Errorf("%w: %w", errClusterUnavailable, err)
	}
	cfg.WrapTransport = func(rt http.RoundTripper) http.RoundTripper {
		return reviewOnlyTransport{next: rt, token: token, origin: origin}
	}
	c, err := authenticationclient.NewForConfig(cfg)
	if err != nil {
		return reviewedUser{}, fmt.Errorf("%w: %w", errClusterUnavailable, err)
	}
	ctx, cancel := context.WithTimeout(ctx, reviewTimeout)
	defer cancel()
	out, err := c.SelfSubjectReviews().Create(ctx, &authenticationv1.SelfSubjectReview{}, metav1.CreateOptions{})
	log := ctrllog.FromContext(ctx)
	switch {
	case err == nil:
		return reviewedUser{Username: out.Status.UserInfo.Username, Groups: out.Status.UserInfo.Groups}, nil
	case apierrors.IsUnauthorized(err), apierrors.IsForbidden(err):
		return reviewedUser{}, fmt.Errorf("%w: the API server refused it", errTokenRefused)
	case apierrors.IsNotFound(err):
		log.Info("Could not validate a token because the API server does not serve authentication.k8s.io/v1 SelfSubjectReview; token sign-in needs Kubernetes 1.28 or later")
		return reviewedUser{}, fmt.Errorf("%w: SelfSubjectReview is not served", errClusterUnavailable)
	default:
		log.Info("Could not reach the API server to validate a token", "error", scrub(err.Error(), token))
		return reviewedUser{}, errClusterUnavailable
	}
}

// reviewOnlyTransport lets the sign-in client send only the
// SelfSubjectReview POST, and only with exactly the token being reviewed and
// no Impersonate-* header, the same credential checks as a token session's
// reads.
type reviewOnlyTransport struct {
	next          http.RoundTripper
	token, origin string
}

// RoundTrip implements http.RoundTripper.
func (t reviewOnlyTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if !sameOrigin(r, t.origin) || r.Method != http.MethodPost || r.URL.Path != reviewPath || r.Header.Get("Upgrade") != "" {
		return nil, fmt.Errorf("%w: %s %s", ErrForbiddenPath, r.Method, r.URL.Path)
	}
	return readOnlyTransport{next: t.next, token: t.token}.roundTripToken(r)
}

// scrub removes token and obvious secret material from text bound for a log.
func scrub(text, token string) string {
	if token != "" {
		text = strings.ReplaceAll(text, token, redact.Placeholder)
	}
	return redact.String(text)
}

// SignInWithToken signs in with a Kubernetes bearer token, for
// POST /auth/token with the form field token. The token is validated with a
// SelfSubjectReview sent with it, and sealed into the session cookie. It
// answers 303 to /apps, to /login?error=token for a token that is not a
// usable credential, and to /login?error=cluster when the API server could
// not judge it. The token is read only from the body, never from the URL,
// and never logged.
func (a *Auth) SignInWithToken(w http.ResponseWriter, r *http.Request) {
	log := ctrllog.FromContext(r.Context())
	r.Body = http.MaxBytesReader(w, r.Body, maxTokenForm)
	if err := r.ParseForm(); err != nil {
		a.refuseToken(w, r, errTokenRefused, "the sign-in form could not be read")
		return
	}
	if r.URL.Query().Has("token") {
		a.refuseToken(w, r, errTokenRefused, "a token was sent in the URL")
		return
	}
	token, err := normalizeToken(r.PostForm.Get("token"))
	if err != nil {
		a.refuseToken(w, r, err, err.Error())
		return
	}
	now := a.now()
	expiry, err := tokenExpiry(token, now)
	if err != nil {
		a.refuseToken(w, r, err, err.Error())
		return
	}
	user, err := a.review(r.Context(), token)
	if err != nil {
		a.refuseToken(w, r, err, scrub(err.Error(), token))
		return
	}
	if err := checkTokenIdentity(user.Username, user.Groups); err != nil {
		a.refuseToken(w, r, errTokenRefused, err.Error())
		return
	}
	id := Identity{Username: user.Username, Groups: user.Groups, Expiry: expiry, Method: MethodToken, Token: Secret(token)}
	sealed, err := seal(a.key, sessionFromIdentity(id))
	if err != nil {
		a.refuseToken(w, r, errClusterUnavailable, "the session could not be sealed")
		return
	}
	if len(sealed) > maxSessionCookie {
		a.refuseToken(w, r, errTokenRefused, "the token and its groups are too large for a session cookie")
		return
	}
	http.SetCookie(w, a.cookie(SessionCookie, sealed, "/", expiry))
	log.Info("Signed in with a Kubernetes token", "user", user.Username, "expires", expiry.UTC().Format(time.RFC3339))
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, "/apps", http.StatusSeeOther)
}

// refuseToken logs why a token sign-in failed, without the token, and
// returns the browser to the login page with the reason.
func (a *Auth) refuseToken(w http.ResponseWriter, r *http.Request, err error, why string) {
	reason := "token"
	if errors.Is(err, errClusterUnavailable) {
		reason = "cluster"
	}
	ctrllog.FromContext(r.Context()).Info("Refused a token sign-in", "reason", reason, "detail", why)
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, "/login?error="+reason, http.StatusSeeOther)
}
