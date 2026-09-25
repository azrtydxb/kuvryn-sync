package console

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"k8s.io/client-go/rest"
)

// TestConsoleStartsWithoutOIDC fails if the console needs OIDC flags to start,
// serves the OIDC routes without them, or accepts only one of the two.
func TestConsoleStartsWithoutOIDC(t *testing.T) {
	// Connectors and an SSO name alone do not turn OIDC on.
	cfg := Config{ClusterName: "test", SSOName: "Dex", Connectors: []string{"github"}}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("a config without OIDC is invalid: %v", err)
	}
	if cfg.OIDCEnabled() {
		t.Fatal("OIDC reported enabled without an issuer or client ID")
	}
	a, err := NewAuth(t.Context(), cfg, &restConfigForTest)
	if err != nil {
		t.Fatalf("the console refused to start without OIDC: %v", err)
	}
	// The self-check must not ask about impersonation, which a token-only
	// console does not use.
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("the self-check sent %s %s without OIDC", r.Method, r.URL.Path)
		http.NotFound(w, r)
	}))
	t.Cleanup(api.Close)
	s, err := NewServer(cfg, &rest.Config{Host: api.URL})
	if err != nil {
		t.Fatal(err)
	}
	s.UseAuthenticator(a)
	if err := s.SelfCheck(t.Context()); err != nil {
		t.Fatal(err)
	}
	h := s.Handler()
	serve := func(method, path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
		return rec
	}
	for _, path := range []string{"/auth/start", "/auth/start?connector=github", "/auth/callback?state=x&code=y"} {
		if rec := serve("GET", path); rec.Code != http.StatusNotFound {
			t.Errorf("GET %s without OIDC = %d, want 404", path, rec.Code)
		}
	}
	me := serve("GET", "/api/me").Body.String()
	for _, want := range []string{`"connectors":[]`, `"tokenSignIn":true`, `"oidc":false`, `"authenticated":false`} {
		if !strings.Contains(me, want) {
			t.Errorf("/api/me without OIDC = %s, want %s", me, want)
		}
	}
	health := serve("GET", "/healthz")
	if health.Code != http.StatusOK || !strings.Contains(health.Body.String(), `"oidc":"disabled"`) || !strings.Contains(health.Body.String(), `"impersonation":"disabled"`) {
		t.Errorf("/healthz without OIDC = %d %s", health.Code, health.Body.String())
	}

	for _, half := range []Config{
		{IssuerURL: "https://dex.example", RedirectURL: "https://console.example/auth/callback"},
		{ClientID: "ksync", RedirectURL: "https://console.example/auth/callback"},
	} {
		if err := half.Validate(); err == nil || !strings.Contains(err.Error(), "set together") {
			t.Errorf("Validate(%+v) = %v, want an error saying both flags go together", half, err)
		}
		if _, err := NewAuth(t.Context(), half, &restConfigForTest); err == nil {
			t.Errorf("NewAuth(%+v) started with only one OIDC flag", half)
		}
	}
	if err := (Config{IssuerURL: "https://dex.example", ClientID: "ksync"}).Validate(); err == nil || !strings.Contains(err.Error(), "--redirect-url") {
		t.Errorf("OIDC without --redirect-url validated: %v", err)
	}
}
