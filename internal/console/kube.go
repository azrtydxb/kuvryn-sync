package console

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	// ErrWriteRefused reports a request other than GET, which the console
	// never sends.
	ErrWriteRefused = errors.New("console: only GET requests are allowed")
	// ErrForbiddenPath reports a GET the console never sends: anything but
	// discovery and resource lists and gets, Secrets, subresources (proxies,
	// exec, attach, port-forward, logs), watches and connection upgrades.
	ErrForbiddenPath = errors.New("console: request path is not allowed")
	// ErrNotImpersonated reports a request that would be sent as the
	// console's own identity instead of the signed-in user's.
	ErrNotImpersonated = errors.New("console: request is not impersonated")
	// ErrNotTheSessionToken reports a token session request that carries
	// anything but the session's own bearer token: another credential, none,
	// or an Impersonate-* header.
	ErrNotTheSessionToken = errors.New("console: request does not carry exactly the session's token")
)

const userClientTimeout = 10 * time.Second

// UserClient returns a read-only client that impersonates id. Every request
// passes through readOnlyTransport after the impersonation headers are set,
// so a request that is not a GET, not impersonated as id, or aimed at a
// Secret is refused before it is sent.
func UserClient(base *rest.Config, scheme *runtime.Scheme, id Identity) (client.Reader, error) {
	if id.IsToken() || id.Token != "" {
		return nil, errors.New("console: a token session is never impersonated")
	}
	if err := checkIdentity(id); err != nil {
		return nil, err
	}
	cfg := rest.CopyConfig(base)
	cfg.Impersonate = rest.ImpersonationConfig{UserName: id.Username, Groups: append([]string(nil), id.Groups...)}
	origin, err := apiOrigin(cfg)
	if err != nil {
		return nil, err
	}
	if cfg.Timeout == 0 || cfg.Timeout > userClientTimeout {
		cfg.Timeout = userClientTimeout
	}
	outer := cfg.WrapTransport
	cfg.WrapTransport = func(rt http.RoundTripper) http.RoundTripper {
		// client-go applies WrapTransport inside the impersonation wrapper, so
		// this is the last check before the request leaves the process.
		rt = readOnlyTransport{next: rt, user: id.Username, origin: origin}
		if outer != nil {
			rt = outer(rt)
		}
		return rt
	}
	return client.New(cfg, client.Options{Scheme: scheme})
}

// tokenConfig returns base's address, CA and client tuning with none of its
// credentials: rest.AnonymousClientConfig copies only an allowlist of safe
// fields, so the console's bearer token and token file, client certificate
// and key, basic auth, impersonation, exec and auth-provider plugins, and any
// custom transport or WrapTransport are all left behind. token becomes the
// only credential.
func tokenConfig(base *rest.Config, token string) *rest.Config {
	cfg := rest.AnonymousClientConfig(base)
	cfg.BearerToken = token
	if cfg.Timeout == 0 || cfg.Timeout > userClientTimeout {
		cfg.Timeout = userClientTimeout
	}
	return cfg
}

// TokenClient returns a read-only client for a token session: every request
// carries id's own bearer token and nothing of the console's, and passes
// through readOnlyTransport in token mode, which refuses anything but a GET
// of an allowed path carrying exactly that token and no Impersonate-* header.
func TokenClient(base *rest.Config, scheme *runtime.Scheme, id Identity) (client.Reader, error) {
	if !id.IsToken() || id.Token == "" {
		return nil, errors.New("console: a token client needs a token session")
	}
	if err := checkTokenIdentity(id.Username, id.Groups); err != nil {
		return nil, err
	}
	token := id.Token.Reveal()
	cfg := tokenConfig(base, token)
	origin, err := apiOrigin(cfg)
	if err != nil {
		return nil, err
	}
	cfg.WrapTransport = func(rt http.RoundTripper) http.RoundTripper {
		// client-go applies WrapTransport inside the bearer-token wrapper, so
		// this sees the final headers just before the request is sent.
		return readOnlyTransport{next: rt, token: token, origin: origin}
	}
	return client.New(cfg, client.Options{Scheme: scheme})
}

// readOnlyTransport sends only GETs for paths other than Secrets and
// workload subresources. Without token it sends only impersonated requests;
// with token only requests carrying exactly that bearer token and no
// impersonation.
type readOnlyTransport struct {
	next http.RoundTripper
	// user, when set, is the only username requests may impersonate.
	user string
	// token, when set, puts the transport in token mode.
	token string
	// origin, when set, is the scheme://host every request must go to. The
	// HTTP client follows redirects and client-go re-adds credentials to each
	// hop, so a redirect elsewhere would otherwise hand that host the token.
	origin string
}

// apiOrigin returns the scheme://host of cfg's API server.
func apiOrigin(cfg *rest.Config) (string, error) {
	u, _, err := rest.DefaultServerUrlFor(cfg)
	if err != nil {
		return "", err
	}
	return u.Scheme + "://" + u.Host, nil
}

// sameOrigin reports whether r goes to origin, or origin is unset.
func sameOrigin(r *http.Request, origin string) bool {
	return origin == "" || r.URL.Scheme+"://"+r.URL.Host == origin
}

// RoundTrip implements http.RoundTripper.
func (t readOnlyTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if !sameOrigin(r, t.origin) {
		return nil, fmt.Errorf("%w: %s is not the API server", ErrForbiddenPath, r.URL.Host)
	}
	if r.Method != http.MethodGet {
		return nil, fmt.Errorf("%w: %s %s", ErrWriteRefused, r.Method, r.URL.Path)
	}
	if forbiddenPath(r.URL.Path) || r.URL.Query().Has("watch") || r.Header.Get("Upgrade") != "" {
		return nil, fmt.Errorf("%w: %s", ErrForbiddenPath, r.URL.Path)
	}
	if t.token != "" {
		return t.roundTripToken(r)
	}
	user := r.Header.Get("Impersonate-User")
	if user == "" || strings.HasPrefix(user, systemPrefix) || (t.user != "" && user != t.user) {
		return nil, ErrNotImpersonated
	}
	for _, g := range r.Header.Values("Impersonate-Group") {
		if strings.HasPrefix(g, systemPrefix) {
			return nil, ErrNotImpersonated
		}
	}
	return t.next.RoundTrip(r)
}

// roundTripToken sends r only when its one credential is the session's
// token and it asks to impersonate no one.
func (t readOnlyTransport) roundTripToken(r *http.Request) (*http.Response, error) {
	for name := range r.Header {
		if strings.HasPrefix(strings.ToLower(name), "impersonate-") {
			return nil, ErrNotTheSessionToken
		}
	}
	auth := r.Header.Values("Authorization")
	if len(auth) != 1 || subtle.ConstantTimeCompare([]byte(auth[0]), []byte("Bearer "+t.token)) != 1 {
		return nil, ErrNotTheSessionToken
	}
	return t.next.RoundTrip(r)
}

// forbiddenPath reports every path the console does not read. It allows
// only API discovery (/api, /api/v1, /apis, /apis/<group>[/<version>]) and
// lists or gets of a resource, namespaced or not, with no subresource. Of
// those it still refuses core Secrets, the legacy /watch/ and /proxy/
// prefixes, and "." or ".." segments. Empty segments are dropped first, so
// "/api/v1//namespaces/a/secrets" is judged as "/api/v1/namespaces/a/secrets".
func forbiddenPath(path string) bool {
	segs := []string{}
	for seg := range strings.SplitSeq(path, "/") {
		switch seg {
		case "":
			continue
		case ".", "..":
			return true
		}
		segs = append(segs, seg)
	}
	var tail []string
	core := false
	switch {
	case len(segs) >= 1 && segs[0] == "api":
		if len(segs) == 1 {
			return false
		}
		tail, core = segs[2:], true
	case len(segs) >= 1 && segs[0] == "apis":
		if len(segs) <= 3 {
			return false
		}
		tail = segs[3:]
	default:
		return true
	}
	if len(tail) == 0 {
		return false
	}
	if tail[0] == "watch" || tail[0] == "proxy" {
		return true
	}
	if tail[0] == "namespaces" && len(tail) >= 3 {
		tail = tail[2:]
	}
	if len(tail) > 2 {
		return true
	}
	return core && tail[0] == "secrets"
}
