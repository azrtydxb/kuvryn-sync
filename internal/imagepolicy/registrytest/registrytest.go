// Package registrytest serves a minimal OCI distribution API for tests.
package registrytest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
)

// Registry is a fake registry holding tag digests for repositories.
type Registry struct {
	*httptest.Server
	Username, Password string

	mu   sync.Mutex
	tags map[string]map[string]string
}

// New starts a registry that requires the given basic credentials.
func New(username, password string) *Registry {
	r := &Registry{Username: username, Password: password, tags: map[string]map[string]string{}}
	r.Server = httptest.NewServer(http.HandlerFunc(r.serve))
	return r
}

// Host is the registry address to use in image references.
func (r *Registry) Host() string {
	return strings.TrimPrefix(r.URL, "http://")
}

// Push records tag with digest in repository.
func (r *Registry) Push(repository, tag, digest string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.tags[repository] == nil {
		r.tags[repository] = map[string]string{}
	}
	r.tags[repository][tag] = digest
}

func (r *Registry) serve(w http.ResponseWriter, req *http.Request) {
	if user, pass, ok := req.BasicAuth(); !ok || user != r.Username || pass != r.Password {
		w.Header().Set("WWW-Authenticate", `Basic realm="registrytest"`)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	path := strings.TrimPrefix(req.URL.Path, "/v2/")
	r.mu.Lock()
	defer r.mu.Unlock()
	switch {
	case strings.HasSuffix(path, "/tags/list"):
		repository := strings.TrimSuffix(path, "/tags/list")
		tags := []string{}
		for tag := range r.tags[repository] {
			tags = append(tags, tag)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"name": repository, "tags": tags})
	case strings.Contains(path, "/manifests/"):
		repository, reference, _ := strings.Cut(path, "/manifests/")
		digest, ok := r.tags[repository][reference]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/vnd.oci.image.manifest.v1+json")
		w.Header().Set("Docker-Content-Digest", digest)
		w.Header().Set("Content-Length", "2")
		if req.Method == http.MethodGet {
			_, _ = w.Write([]byte("{}"))
		}
	default:
		w.WriteHeader(http.StatusOK)
	}
}
