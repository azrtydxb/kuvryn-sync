package console

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"sync/atomic"
	"time"

	"k8s.io/client-go/rest"

	"github.com/azrtydxb/kuvryn-sync/internal/console/ui"
)

// ContentSecurityPolicy forbids inline script and style and any framing.
const ContentSecurityPolicy = "default-src 'self'; img-src 'self' data:; style-src 'self'; script-src 'self'; frame-ancestors 'none'"

// Server serves the console's routes.
type Server struct {
	cfg   Config
	base  *rest.Config
	files fs.FS

	oidcReady     atomic.Bool
	impersonation atomic.Value // string: unchecked, granted or missing
}

// NewServer builds the console server. base is the console's own cluster
// configuration; it is only ever used to impersonate signed-in users.
func NewServer(cfg Config, base *rest.Config) (*Server, error) {
	if base == nil {
		return nil, errors.New("console: a cluster configuration is required")
	}
	s := &Server{cfg: cfg, base: rest.CopyConfig(base), files: ui.Files()}
	s.impersonation.Store("unchecked")
	return s, nil
}

// Handler returns the console's HTTP handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.healthz)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	})
	mux.HandleFunc("/", s.spa)
	return securityHeaders(mux)
}

// Run serves the console on cfg.Listen until ctx is done.
func (s *Server) Run(ctx context.Context) error {
	srv := &http.Server{
		Addr:              s.cfg.Listen,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdown)
	}
}

func (s *Server) healthz(w http.ResponseWriter, _ *http.Request) {
	oidc := "pending"
	if s.oidcReady.Load() {
		oidc = "ready"
	}
	impersonation, _ := s.impersonation.Load().(string)
	code := http.StatusOK
	if oidc != "ready" {
		code = http.StatusServiceUnavailable
	}
	writeJSON(w, code, map[string]string{"oidc": oidc, "impersonation": impersonation})
}

// spa serves the SPA's files, long-cached under /assets/, and index.html for
// every other path so client-side routes load.
func (s *Server) spa(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if name != "" && name != "index.html" {
		if info, err := fs.Stat(s.files, name); err == nil && !info.IsDir() {
			if strings.HasPrefix(name, "assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			http.ServeFileFS(w, r, s.files, name)
			return
		}
		if strings.HasPrefix(name, "assets/") {
			http.NotFound(w, r)
			return
		}
	}
	index, err := fs.ReadFile(s.files, "index.html")
	if err != nil {
		http.Error(w, "console UI missing", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(index)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", ContentSecurityPolicy)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
