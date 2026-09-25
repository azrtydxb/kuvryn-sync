package console

import (
	"net/http/httptest"
	"strings"
	"testing"

	"k8s.io/client-go/rest"
)

func TestServerServesHealthAndSecurityHeaders(t *testing.T) {
	s, err := NewServer(Config{ClusterName: "test"}, &rest.Config{Host: "https://127.0.0.1:1"})
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/apps", nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "<html") {
		t.Fatalf("SPA fallback: %d %s", rec.Code, rec.Body.String())
	}
	const csp = "default-src 'self'; img-src 'self' data:; style-src 'self'; script-src 'self'; frame-ancestors 'none'; form-action 'self'; base-uri 'none'"
	if got := rec.Header().Get("Content-Security-Policy"); got != csp {
		t.Fatalf("CSP = %q, want %q", got, csp)
	}
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/healthz", nil))
	if rec.Code != 503 {
		t.Fatalf("healthz before OIDC discovery = %d, want 503", rec.Code)
	}
}

var restConfigForTest = rest.Config{Host: "https://127.0.0.1:1"}

func TestSPARefusesPathTraversal(t *testing.T) {
	s, err := NewServer(Config{}, &restConfigForTest)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"/../ui/embed.go", "/assets/../../server.go", "/%2e%2e/embed.go"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/", nil)
		req.URL.Path = p
		s.Handler().ServeHTTP(rec, req)
		if strings.Contains(rec.Body.String(), "package ") {
			t.Fatalf("%s served a source file: %s", p, rec.Body.String())
		}
	}
}
