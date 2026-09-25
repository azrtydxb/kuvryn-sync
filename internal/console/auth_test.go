package console

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	jose "github.com/go-jose/go-jose/v4"
)

func TestOIDCLoginFlow(t *testing.T) {
	iss := newTestIssuer(t) // serves /.well-known/openid-configuration, /keys, /token
	a := mustAuth(t, iss.URL)
	start := httptest.NewRecorder()
	a.Start(start, httptest.NewRequest("GET", "/auth/start?connector=github", nil))
	loc, _ := url.Parse(start.Header().Get("Location"))
	q := loc.Query()
	if q.Get("code_challenge_method") != "S256" || q.Get("connector_id") != "github" || q.Get("state") == "" || q.Get("nonce") == "" {
		t.Fatalf("authorize URL lacks PKCE/state/nonce/connector: %s", loc)
	}
	iss.issueFor(q.Get("nonce"), "alice@acme.io", []string{"team-a"})
	cb := httptest.NewRequest("GET", "/auth/callback?code=c&state="+q.Get("state"), nil)
	for _, c := range start.Result().Cookies() {
		cb.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	a.Callback(rec, cb)
	var session *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == "ksync_session" {
			session = c
		}
	}
	if session == nil || !session.HttpOnly || !session.Secure || session.SameSite != http.SameSiteLaxMode {
		t.Fatalf("session cookie = %+v", session)
	}
	bad := httptest.NewRequest("GET", "/auth/callback?code=c&state=wrong", nil)
	for _, c := range start.Result().Cookies() {
		bad.AddCookie(c)
	}
	rec = httptest.NewRecorder()
	a.Callback(rec, bad)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("wrong state accepted: %d", rec.Code)
	}
	iss.issueFor("other-nonce", "alice@acme.io", nil)
	rec = httptest.NewRecorder()
	a.Callback(rec, cb)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("wrong nonce accepted: %d", rec.Code)
	}
	iss.issueUnsigned("alice@acme.io")
	rec = httptest.NewRecorder()
	a.Callback(rec, cb)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unverifiable ID token accepted: %d", rec.Code)
	}
}

const testClientID = "ksync"

// testIssuer is an in-process OIDC issuer: discovery, JWKS and a token
// endpoint that returns the ID token set by issueFor or issueUnsigned.
type testIssuer struct {
	*httptest.Server
	t       *testing.T
	key     *rsa.PrivateKey
	mu      sync.Mutex
	idToken string
}

func newTestIssuer(t *testing.T) *testIssuer {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	iss := &testIssuer{t: t, key: key}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                iss.URL,
			"authorization_endpoint":                iss.URL + "/auth",
			"token_endpoint":                        iss.URL + "/token",
			"jwks_uri":                              iss.URL + "/keys",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("/keys", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: "k1", Algorithm: "RS256", Use: "sig"}}})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil || r.PostForm.Get("grant_type") != "authorization_code" || len(r.PostForm.Get("code_verifier")) < 43 {
			http.Error(w, `{"error":"invalid_request"}`, http.StatusBadRequest)
			return
		}
		iss.mu.Lock()
		defer iss.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "a", "token_type": "Bearer", "expires_in": 3600, "id_token": iss.idToken})
	})
	iss.Server = httptest.NewServer(mux)
	t.Cleanup(iss.Close)
	return iss
}

func (iss *testIssuer) claims(nonce string, extra map[string]any) map[string]any {
	c := map[string]any{
		"iss":   iss.URL,
		"sub":   "u1",
		"aud":   testClientID,
		"iat":   time.Now().Unix(),
		"exp":   time.Now().Add(time.Hour).Unix(),
		"nonce": nonce,
	}
	for k, v := range extra {
		c[k] = v
	}
	return c
}

// issueFor makes the token endpoint return an ID token signed by the issuer.
func (iss *testIssuer) issueFor(nonce, email string, groups []string) {
	extra := map[string]any{"email": email, "email_verified": true}
	if groups != nil {
		extra["groups"] = groups
	}
	iss.issueClaims(iss.key, iss.claims(nonce, extra))
}

func (iss *testIssuer) issueClaims(key *rsa.PrivateKey, claims map[string]any) {
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: jose.JSONWebKey{Key: key, KeyID: "k1"}}, nil)
	if err != nil {
		iss.t.Fatal(err)
	}
	payload, _ := json.Marshal(claims)
	jws, err := signer.Sign(payload)
	if err != nil {
		iss.t.Fatal(err)
	}
	raw, err := jws.CompactSerialize()
	if err != nil {
		iss.t.Fatal(err)
	}
	iss.mu.Lock()
	iss.idToken = raw
	iss.mu.Unlock()
}

// issueUnsigned makes the token endpoint return an "alg: none" ID token.
func (iss *testIssuer) issueUnsigned(email string) {
	enc := base64.RawURLEncoding.EncodeToString
	payload, _ := json.Marshal(iss.claims("", map[string]any{"email": email}))
	iss.mu.Lock()
	iss.idToken = enc([]byte(`{"alg":"none","typ":"JWT"}`)) + "." + enc(payload) + "."
	iss.mu.Unlock()
}

func testConfig(t *testing.T, issuerURL string) Config {
	t.Helper()
	keyFile := filepath.Join(t.TempDir(), "session.key")
	key := make([]byte, SessionKeySize)
	_, _ = rand.Read(key)
	if err := os.WriteFile(keyFile, key, 0o600); err != nil {
		t.Fatal(err)
	}
	return Config{
		IssuerURL:      issuerURL,
		ClientID:       testClientID,
		RedirectURL:    "https://console.example/auth/callback",
		UsernameClaim:  "email",
		GroupsClaim:    "groups",
		SessionKeyFile: keyFile,
		ClusterName:    "test",
		Connectors:     []string{"github", "gitlab", "local"},
	}
}

func mustAuth(t *testing.T, issuerURL string) *Auth {
	t.Helper()
	return mustAuthWith(t, testConfig(t, issuerURL))
}

func mustAuthWith(t *testing.T, cfg Config) *Auth {
	t.Helper()
	a, err := NewAuth(context.Background(), cfg, &restConfigForTest)
	if err != nil {
		t.Fatal(err)
	}
	if !a.Ready() {
		t.Fatal("OIDC discovery against the test issuer failed")
	}
	return a
}

// signIn runs Start and Callback, returning the callback response.
func signIn(t *testing.T, a *Auth, iss *testIssuer, issue func(nonce string)) *httptest.ResponseRecorder {
	t.Helper()
	start := httptest.NewRecorder()
	a.Start(start, httptest.NewRequest("GET", "/auth/start", nil))
	loc, err := url.Parse(start.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	issue(loc.Query().Get("nonce"))
	cb := httptest.NewRequest("GET", "/auth/callback?code=c&state="+loc.Query().Get("state"), nil)
	for _, c := range start.Result().Cookies() {
		cb.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	a.Callback(rec, cb)
	return rec
}

func sessionCookie(rec *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == SessionCookie && c.Value != "" {
			return c
		}
	}
	return nil
}

func TestOIDCSessionIdentityAndRefusals(t *testing.T) {
	iss := newTestIssuer(t)
	a := mustAuth(t, iss.URL)

	rec := signIn(t, a, iss, func(n string) { iss.issueFor(n, "alice@acme.io", []string{"team-a"}) })
	cookie := sessionCookie(rec)
	if rec.Code != http.StatusSeeOther || cookie == nil {
		t.Fatalf("sign-in = %d, cookie %v", rec.Code, cookie)
	}
	if time.Until(cookie.Expires) > time.Hour+time.Minute || time.Until(cookie.Expires) < 50*time.Minute {
		t.Fatalf("session expiry %v does not follow the ID token exp", cookie.Expires)
	}
	req := httptest.NewRequest("GET", "/api/me", nil)
	req.AddCookie(cookie)
	id, err := a.Identity(req)
	if err != nil || id.Username != "alice@acme.io" || strings.Join(id.Groups, ",") != "team-a" {
		t.Fatalf("Identity = %+v, %v", id, err)
	}
	if strings.Contains(cookie.Value, "alice") {
		t.Fatal("session cookie is not encrypted")
	}

	// A rotated key logs everyone out.
	rotated := mustAuth(t, iss.URL)
	if _, err := rotated.Identity(req); err != ErrNoSession {
		t.Fatalf("session survived a key rotation: %v", err)
	}
	// An expired session is refused.
	a.now = func() time.Time { return time.Now().Add(2 * time.Hour) }
	if _, err := a.Identity(req); err != ErrNoSession {
		t.Fatalf("expired session accepted: %v", err)
	}
	a.now = time.Now

	cases := []struct {
		name  string
		issue func(nonce string)
		code  int
		body  string
	}{
		{"system user", func(n string) { iss.issueFor(n, "system:admin", nil) }, http.StatusForbidden, "system:"},
		{"system group", func(n string) { iss.issueFor(n, "bob@acme.io", []string{"system:masters"}) }, http.StatusForbidden, "system:"},
		{"missing username claim", func(n string) { iss.issueClaims(iss.key, iss.claims(n, nil)) }, http.StatusBadRequest, "email"},
		{"unverified email", func(n string) {
			iss.issueClaims(iss.key, iss.claims(n, map[string]any{"email": "eve@acme.io", "email_verified": false}))
		}, http.StatusForbidden, "not verified"},
		{"foreign signing key", func(n string) {
			other, _ := rsa.GenerateKey(rand.Reader, 2048)
			iss.issueClaims(other, iss.claims(n, map[string]any{"email": "eve@acme.io"}))
		}, http.StatusBadRequest, "verified"},
		{"expired token", func(n string) {
			c := iss.claims(n, map[string]any{"email": "eve@acme.io"})
			c["exp"] = time.Now().Add(-time.Minute).Unix()
			iss.issueClaims(iss.key, c)
		}, http.StatusBadRequest, "expired"},
		{"wrong audience", func(n string) {
			c := iss.claims(n, map[string]any{"email": "eve@acme.io"})
			c["aud"] = "someone-else"
			iss.issueClaims(iss.key, c)
		}, http.StatusBadRequest, "audience"},
		{"too many groups", func(n string) {
			groups := make([]string, 500)
			for i := range groups {
				groups[i] = fmt.Sprintf("group-with-a-long-name-%04d", i)
			}
			iss.issueFor(n, "bob@acme.io", groups)
		}, http.StatusBadRequest, "too many groups for a session"},
	}
	for _, tc := range cases {
		rec := signIn(t, a, iss, tc.issue)
		if rec.Code != tc.code || !strings.Contains(rec.Body.String(), tc.body) || sessionCookie(rec) != nil {
			t.Errorf("%s: %d %s", tc.name, rec.Code, rec.Body.String())
		}
	}

	// Prefixes apply to the username and every group.
	cfg := testConfig(t, iss.URL)
	cfg.UsernamePrefix, cfg.GroupsPrefix = "oidc:", "oidc:"
	prefixed := mustAuthWith(t, cfg)
	rec = signIn(t, prefixed, iss, func(n string) { iss.issueFor(n, "carol@acme.io", []string{"ops"}) })
	req = httptest.NewRequest("GET", "/", nil)
	req.AddCookie(sessionCookie(rec))
	if id, err := prefixed.Identity(req); err != nil || id.Username != "oidc:carol@acme.io" || id.Groups[0] != "oidc:ops" {
		t.Fatalf("prefixed identity = %+v, %v", id, err)
	}

	// An unknown connector is refused, and logout clears the session.
	start := httptest.NewRecorder()
	a.Start(start, httptest.NewRequest("GET", "/auth/start?connector=evil", nil))
	if start.Code != http.StatusBadRequest {
		t.Fatalf("unknown connector = %d", start.Code)
	}
	out := httptest.NewRecorder()
	a.Logout(out, httptest.NewRequest("POST", "/logout", nil))
	if c := out.Result().Cookies(); len(c) != 1 || c[0].Name != SessionCookie || c[0].MaxAge >= 0 {
		t.Fatalf("logout cookies = %+v", c)
	}
}

func TestServerReportsOIDCReadinessAndRoutesSignIn(t *testing.T) {
	iss := newTestIssuer(t)
	a := mustAuth(t, iss.URL)
	s, err := NewServer(Config{ClusterName: "test", SSOName: "Dex", Connectors: []string{"github"}}, &restConfigForTest)
	if err != nil {
		t.Fatal(err)
	}
	s.UseAuthenticator(a)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/healthz", nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"oidc":"ready"`) {
		t.Fatalf("healthz after discovery = %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/api/me", nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"authenticated":false`) || !strings.Contains(rec.Body.String(), `"cluster":"test"`) ||
		!strings.Contains(rec.Body.String(), `"ssoName":"Dex"`) || !strings.Contains(rec.Body.String(), `"connectors":["github"]`) || strings.Contains(rec.Body.String(), "username") {
		t.Fatalf("/api/me without a session = %d %s", rec.Code, rec.Body.String())
	}
	cb := signIn(t, a, iss, func(n string) { iss.issueFor(n, "alice@acme.io", []string{"team-a"}) })
	req := httptest.NewRequest("GET", "/api/me", nil)
	req.AddCookie(sessionCookie(cb))
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"username":"alice@acme.io"`) || !strings.Contains(rec.Body.String(), `"cluster":"test"`) {
		t.Fatalf("/api/me = %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/logout", nil))
	if len(rec.Result().Cookies()) != 0 {
		t.Fatalf("GET /logout touched cookies; only POST may sign out: %+v", rec.Result().Cookies())
	}
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest("POST", "/logout", nil))
	if rec.Code != http.StatusSeeOther || sessionCookie(rec) != nil || len(rec.Result().Cookies()) != 1 {
		t.Fatalf("POST /logout = %d %+v", rec.Code, rec.Result().Cookies())
	}
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/auth/start", nil))
	if rec.Code != http.StatusFound {
		t.Fatalf("/auth/start = %d", rec.Code)
	}
}

func TestDiscoveryRetriesUntilTheIssuerAnswers(t *testing.T) {
	cfg := testConfig(t, "http://127.0.0.1:1")
	a, err := NewAuth(t.Context(), cfg, &restConfigForTest)
	if err != nil {
		t.Fatalf("an unreachable issuer failed startup: %v", err)
	}
	if a.Ready() {
		t.Fatal("ready without discovery")
	}
	rec := httptest.NewRecorder()
	a.Start(rec, httptest.NewRequest("GET", "/auth/start", nil))
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("start before discovery = %d", rec.Code)
	}
}

func TestWithoutASessionKeyFileTheKeyIsEphemeral(t *testing.T) {
	iss := newTestIssuer(t)
	cfg := testConfig(t, iss.URL)
	cfg.SessionKeyFile = ""
	a := mustAuthWith(t, cfg)
	rec := signIn(t, a, iss, func(n string) { iss.issueFor(n, "alice@acme.io", nil) })
	cookie := sessionCookie(rec)
	if cookie == nil {
		t.Fatalf("no session with an in-memory key: %d %s", rec.Code, rec.Body.String())
	}
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(cookie)
	if id, err := a.Identity(req); err != nil || id.Username != "alice@acme.io" {
		t.Fatalf("Identity = %+v, %v", id, err)
	}
	// Another process, such as a restart or a second replica, has its own key.
	if _, err := mustAuthWith(t, cfg).Identity(req); err != ErrNoSession {
		t.Fatalf("a session survived into a new process: %v", err)
	}
}

// startCookies begins a sign-in and returns its cookies and state.
func startCookies(t *testing.T, a *Auth) ([]*http.Cookie, string) {
	t.Helper()
	start := httptest.NewRecorder()
	a.Start(start, httptest.NewRequest("GET", "/auth/start", nil))
	loc, err := url.Parse(start.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	return start.Result().Cookies(), loc.Query().Get("state")
}

func TestCallbackErrorsShowOnlyFixedText(t *testing.T) {
	iss := newTestIssuer(t)
	a := mustAuth(t, iss.URL)
	cookies, state := startCookies(t, a)
	callback := func(query string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", "/auth/callback?"+query, nil)
		for _, c := range cookies {
			req.AddCookie(c)
		}
		rec := httptest.NewRecorder()
		a.Callback(rec, req)
		return rec
	}
	evil := url.QueryEscape("<b>Your account is locked, call +1 555 0100</b>")
	// Without the right state, an error is not even looked at.
	rec := callback("error=access_denied&error_description=" + evil + "&state=wrong")
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "error=state") || strings.Contains(rec.Body.String(), "locked") {
		t.Fatalf("error with a wrong state = %d %s", rec.Code, rec.Body.String())
	}
	rec = callback("error=access_denied&error_description=" + evil + "&state=" + state)
	if !strings.Contains(rec.Body.String(), "error=denied") || !strings.Contains(rec.Body.String(), "The identity provider denied the sign-in.") || strings.Contains(rec.Body.String(), "locked") {
		t.Fatalf("access_denied = %d %s", rec.Code, rec.Body.String())
	}
	rec = callback("error=" + evil + "&state=" + state)
	if !strings.Contains(rec.Body.String(), "Sign-in failed.") || strings.Contains(rec.Body.String(), "locked") {
		t.Fatalf("unknown error = %d %s", rec.Code, rec.Body.String())
	}
}

func TestEmailVerifiedMustBeTrue(t *testing.T) {
	iss := newTestIssuer(t)
	a := mustAuth(t, iss.URL)
	for _, verified := range []any{"false", "true", 0, false} {
		rec := signIn(t, a, iss, func(n string) {
			iss.issueClaims(iss.key, iss.claims(n, map[string]any{"email": "eve@acme.io", "email_verified": verified}))
		})
		if rec.Code != http.StatusForbidden || sessionCookie(rec) != nil {
			t.Errorf("email_verified %#v = %d, want 403", verified, rec.Code)
		}
	}
}

func TestSignInUsesOneStateCookie(t *testing.T) {
	iss := newTestIssuer(t)
	cookies, _ := startCookies(t, mustAuth(t, iss.URL))
	if len(cookies) != 1 || cookies[0].Name != StateCookie {
		t.Fatalf("sign-in cookies = %+v, want only %s", cookies, StateCookie)
	}
}

func TestLogoutRefusesCrossSiteRequests(t *testing.T) {
	iss := newTestIssuer(t)
	s, err := NewServer(Config{ClusterName: "test"}, &restConfigForTest)
	if err != nil {
		t.Fatal(err)
	}
	s.UseAuthenticator(mustAuth(t, iss.URL))
	for _, tc := range []struct {
		name   string
		header map[string]string
		want   int
	}{
		{"cross-site fetch", map[string]string{"Sec-Fetch-Site": "cross-site"}, http.StatusForbidden},
		{"foreign origin", map[string]string{"Origin": "https://evil.example"}, http.StatusForbidden},
		{"same-origin fetch", map[string]string{"Sec-Fetch-Site": "same-origin"}, http.StatusSeeOther},
		{"same origin", map[string]string{"Origin": "https://console.example"}, http.StatusSeeOther},
	} {
		req := httptest.NewRequest("POST", "https://console.example/logout", nil)
		for k, v := range tc.header {
			req.Header.Set(k, v)
		}
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != tc.want {
			t.Errorf("%s: POST /logout = %d, want %d", tc.name, rec.Code, tc.want)
		}
		if tc.want == http.StatusForbidden && len(rec.Result().Cookies()) != 0 {
			t.Errorf("%s: a refused logout still touched cookies", tc.name)
		}
	}
}

func TestInsecureCookiesOnlyForLocalhost(t *testing.T) {
	iss := newTestIssuer(t)
	for redirect, ok := range map[string]bool{
		"https://console.example/auth/callback":       false,
		"http://10.0.0.5:8080/auth/callback":          false,
		"http://localhost.evil.example/auth/callback": false,
		"http://localhost:5174/auth/callback":         true,
		"http://127.0.0.1:8080/auth/callback":         true,
		"http://[::1]:8080/auth/callback":             true,
	} {
		cfg := testConfig(t, iss.URL)
		cfg.InsecureCookies, cfg.RedirectURL = true, redirect
		_, err := NewAuth(context.Background(), cfg, &restConfigForTest)
		if (err == nil) != ok {
			t.Errorf("--insecure-cookies with %s: err = %v", redirect, err)
		}
	}
}

// Without OIDC there is no redirect URL, so --insecure-cookies is judged by
// the address the console listens on.
func TestInsecureCookiesWithoutOIDCOnlyOnLoopback(t *testing.T) {
	for listen, ok := range map[string]bool{
		":8080":          false,
		"0.0.0.0:8080":   false,
		"10.0.0.5:8080":  false,
		"127.0.0.1:5174": true,
		"localhost:5174": true,
		"[::1]:5174":     true,
	} {
		err := Config{Listen: listen, InsecureCookies: true}.Validate()
		if (err == nil) != ok {
			t.Errorf("--insecure-cookies with --listen %s: err = %v", listen, err)
		}
	}
}
