package console

import (
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
)

const userClientTimeout = 10 * time.Second

// UserClient returns a read-only client that impersonates id. Every request
// passes through readOnlyTransport after the impersonation headers are set,
// so a request that is not a GET, not impersonated as id, or aimed at a
// Secret is refused before it is sent.
func UserClient(base *rest.Config, scheme *runtime.Scheme, id Identity) (client.Reader, error) {
	if err := checkIdentity(id); err != nil {
		return nil, err
	}
	cfg := rest.CopyConfig(base)
	cfg.Impersonate = rest.ImpersonationConfig{UserName: id.Username, Groups: append([]string(nil), id.Groups...)}
	if cfg.Timeout == 0 || cfg.Timeout > userClientTimeout {
		cfg.Timeout = userClientTimeout
	}
	outer := cfg.WrapTransport
	cfg.WrapTransport = func(rt http.RoundTripper) http.RoundTripper {
		// client-go applies WrapTransport inside the impersonation wrapper, so
		// this is the last check before the request leaves the process.
		rt = readOnlyTransport{next: rt, user: id.Username}
		if outer != nil {
			rt = outer(rt)
		}
		return rt
	}
	return client.New(cfg, client.Options{Scheme: scheme})
}

// readOnlyTransport sends only impersonated GETs for paths other than
// Secrets and workload subresources.
type readOnlyTransport struct {
	next http.RoundTripper
	// user, when set, is the only username requests may impersonate.
	user string
}

// RoundTrip implements http.RoundTripper.
func (t readOnlyTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.Method != http.MethodGet {
		return nil, fmt.Errorf("%w: %s %s", ErrWriteRefused, r.Method, r.URL.Path)
	}
	if forbiddenPath(r.URL.Path) || r.URL.Query().Has("watch") || r.Header.Get("Upgrade") != "" {
		return nil, fmt.Errorf("%w: %s", ErrForbiddenPath, r.URL.Path)
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
