// Package registrytest serves a minimal OCI distribution API for tests.
package registrytest

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
)

// Registry is a fake registry holding tag digests for repositories.
type Registry struct {
	*httptest.Server
	Username, Password string

	mu        sync.Mutex
	tags      map[string]map[string]string
	manifests map[string][]byte
	blobs     map[string][]byte
}

// New starts a registry that requires the given basic credentials.
func New(username, password string) *Registry {
	r := &Registry{Username: username, Password: password, tags: map[string]map[string]string{}, manifests: map[string][]byte{}, blobs: map[string][]byte{}}
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

// PushArtifact stores an OCI artifact with one config and one layer under tag
// and returns the manifest digest.
func (r *Registry) PushArtifact(repository, tag, configType string, config []byte, layerType string, layer []byte) string {
	manifest, _ := json.Marshal(map[string]any{
		"schemaVersion": 2,
		"mediaType":     "application/vnd.oci.image.manifest.v1+json",
		"config":        descriptor(configType, config),
		"layers":        []any{descriptor(layerType, layer)},
	})
	digest := digestOf(manifest)
	r.mu.Lock()
	r.blobs[digestOf(config)], r.blobs[digestOf(layer)], r.manifests[digest] = config, layer, manifest
	r.mu.Unlock()
	r.Push(repository, tag, digest)
	return digest
}

func descriptor(mediaType string, data []byte) map[string]any {
	return map[string]any{"mediaType": mediaType, "digest": digestOf(data), "size": len(data)}
}

func digestOf(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
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
		if !ok && strings.HasPrefix(reference, "sha256:") {
			digest, ok = reference, r.manifests[reference] != nil
		}
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		body, stored := r.manifests[digest]
		if !stored {
			body = []byte("{}")
		}
		w.Header().Set("Content-Type", "application/vnd.oci.image.manifest.v1+json")
		w.Header().Set("Docker-Content-Digest", digest)
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		if req.Method == http.MethodGet {
			_, _ = w.Write(body)
		}
	case strings.Contains(path, "/blobs/"):
		_, digest, _ := strings.Cut(path, "/blobs/")
		blob, ok := r.blobs[digest]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Docker-Content-Digest", digest)
		w.Header().Set("Content-Length", strconv.Itoa(len(blob)))
		if req.Method == http.MethodGet {
			_, _ = w.Write(blob)
		}
	default:
		w.WriteHeader(http.StatusOK)
	}
}
