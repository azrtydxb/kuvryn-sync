package console

import (
	"bytes"
	"context"
	"crypto/rand"
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
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-logr/logr/funcr"
	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
)

// tokenConfigForTest is a token-only console with a fixed session key.
func tokenConfigForTest(t *testing.T) Config {
	t.Helper()
	keyFile := filepath.Join(t.TempDir(), "session.key")
	key := make([]byte, SessionKeySize)
	_, _ = rand.Read(key)
	if err := os.WriteFile(keyFile, key, 0o600); err != nil {
		t.Fatal(err)
	}
	return Config{ClusterName: "test", SessionKeyFile: keyFile}
}

// tokenServer returns a token-only console reading the cluster at base.
func tokenServer(t *testing.T, base *rest.Config) (*Server, *Auth) {
	t.Helper()
	cfg := tokenConfigForTest(t)
	a, err := NewAuth(t.Context(), cfg, base)
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewServer(cfg, base)
	if err != nil {
		t.Fatal(err)
	}
	s.UseAuthenticator(a)
	return s, a
}

// mintToken creates the ServiceAccount ns/name if needed and returns a token
// for it that lasts ttl.
func mintToken(t *testing.T, ns, name string, ttl time.Duration) string {
	t.Helper()
	cs, err := kubernetes.NewForConfig(env.Config)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := cs.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}}, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatal(err)
	}
	if _, err := cs.CoreV1().ServiceAccounts(ns).Create(ctx, &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: name}}, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatal(err)
	}
	seconds := int64(ttl / time.Second)
	tr, err := cs.CoreV1().ServiceAccounts(ns).CreateToken(ctx, name, &authenticationv1.TokenRequest{
		Spec: authenticationv1.TokenRequestSpec{ExpirationSeconds: &seconds},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return tr.Status.Token
}

// postToken posts a token sign-in form from the console's own origin.
func postToken(h http.Handler, token string, ctx context.Context) *httptest.ResponseRecorder {
	req := httptest.NewRequestWithContext(ctx, "POST", "/auth/token", strings.NewReader(url.Values{"token": {token}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// unsignedJWT builds a JWT-shaped token with the given exp claim; the
// signature is not valid, which only the API server can judge.
func unsignedJWT(exp time.Time) string {
	enc := base64.RawURLEncoding
	payload, _ := json.Marshal(map[string]any{"sub": "system:serviceaccount:a:viewer", "exp": exp.Unix()})
	return enc.EncodeToString([]byte(`{"alg":"RS256"}`)) + "." + enc.EncodeToString(payload) + "." + enc.EncodeToString([]byte("sig"))
}

// fakeReviewServer answers SelfSubjectReviews with user, or with status when
// it is not 200, and counts the requests it gets.
func fakeReviewServer(t *testing.T, status int, user authenticationv1.UserInfo) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodPost || r.URL.Path != "/apis/authentication.k8s.io/v1/selfsubjectreviews" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		if status != http.StatusOK {
			w.WriteHeader(status)
			_, _ = fmt.Fprintf(w, `{"kind":"Status","apiVersion":"v1","status":"Failure","code":%d}`, status)
			return
		}
		_ = json.NewEncoder(w).Encode(authenticationv1.SelfSubjectReview{
			TypeMeta: metav1.TypeMeta{APIVersion: "authentication.k8s.io/v1", Kind: "SelfSubjectReview"},
			Status:   authenticationv1.SelfSubjectReviewStatus{UserInfo: user},
		})
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func sessionOf(rec *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == SessionCookie && c.Value != "" {
			return c
		}
	}
	return nil
}

func getWith(h http.Handler, path string, c *http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", path, nil)
	if c != nil {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// TestTokenSignIn fails if a valid ServiceAccount token does not sign in as
// that ServiceAccount, or if an anonymous, expired, malformed or oversized
// token does.
func TestTokenSignIn(t *testing.T) {
	s, _ := tokenServer(t, env.Config)
	h := s.Handler()
	token := mintToken(t, "a", "viewer", time.Hour)

	rec := postToken(h, "  Bearer "+token+"\n", context.Background())
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/apps" {
		t.Fatalf("valid token = %d to %q, want 303 to /apps", rec.Code, rec.Header().Get("Location"))
	}
	cookie := sessionOf(rec)
	if cookie == nil || !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteLaxMode || cookie.Path != "/" {
		t.Fatalf("session cookie = %+v, want HttpOnly, Secure, SameSite=Lax on /", cookie)
	}
	me := getWith(h, "/api/me", cookie).Body.String()
	for _, want := range []string{`"authenticated":true`, `"username":"system:serviceaccount:a:viewer"`, `"method":"token"`} {
		if !strings.Contains(me, want) {
			t.Errorf("/api/me = %s, want %s", me, want)
		}
	}

	refused := map[string]string{
		"garbage":            "not-a-token",
		"forged JWT":         unsignedJWT(time.Now().Add(time.Hour)),
		"expired JWT":        unsignedJWT(time.Now().Add(-time.Minute)),
		"oversized":          strings.Repeat("a", 16<<10+1),
		"space inside":       token[:10] + " " + token[10:],
		"empty":              "   ",
		"only a Bearer word": "Bearer ",
	}
	for name, tok := range refused {
		rec := postToken(h, tok, context.Background())
		if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/login?error=token" || sessionOf(rec) != nil {
			t.Errorf("%s: %d to %q with cookie %v, want 303 to /login?error=token and no session", name, rec.Code, rec.Header().Get("Location"), sessionOf(rec) != nil)
		}
	}

	// Refusals that must happen before any request reaches the API server.
	api, calls := fakeReviewServer(t, http.StatusOK, authenticationv1.UserInfo{Username: "system:serviceaccount:a:viewer"})
	fake, _ := tokenServer(t, &rest.Config{Host: api.URL})
	for _, tok := range []string{strings.Repeat("a", 16<<10+1), unsignedJWT(time.Now().Add(-time.Minute)), "a\x00b"} {
		postToken(fake.Handler(), tok, context.Background())
	}
	if n := calls.Load(); n != 0 {
		t.Errorf("%d reviews were sent for tokens refused locally", n)
	}

	// The API server's answer decides the rest.
	for name, tc := range map[string]struct {
		status int
		user   authenticationv1.UserInfo
		want   string
	}{
		"anonymous":             {200, authenticationv1.UserInfo{Username: "system:anonymous", Groups: []string{"system:unauthenticated"}}, "/login?error=token"},
		"unauthenticated":       {200, authenticationv1.UserInfo{Username: "someone", Groups: []string{"system:unauthenticated"}}, "/login?error=token"},
		"a node":                {200, authenticationv1.UserInfo{Username: "system:node:worker-1", Groups: []string{"system:nodes"}}, "/login?error=token"},
		"no username":           {200, authenticationv1.UserInfo{}, "/login?error=token"},
		"rejected token":        {401, authenticationv1.UserInfo{}, "/login?error=token"},
		"not served (pre-1.28)": {404, authenticationv1.UserInfo{}, "/login?error=cluster"},
		"server error":          {500, authenticationv1.UserInfo{}, "/login?error=cluster"},
		"a person":              {200, authenticationv1.UserInfo{Username: "mia@acme.io", Groups: []string{"acme:platform", "system:authenticated"}}, "/apps"},
	} {
		api, _ := fakeReviewServer(t, tc.status, tc.user)
		srv, _ := tokenServer(t, &rest.Config{Host: api.URL})
		rec := postToken(srv.Handler(), "opaque-static-token", context.Background())
		if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != tc.want || (sessionOf(rec) != nil) != (tc.want == "/apps") {
			t.Errorf("%s: %d to %q (session %v), want 303 to %s", name, rec.Code, rec.Header().Get("Location"), sessionOf(rec) != nil, tc.want)
		}
	}
	down, _ := tokenServer(t, &rest.Config{Host: "https://127.0.0.1:1"})
	if rec := postToken(down.Handler(), "opaque-static-token", context.Background()); rec.Header().Get("Location") != "/login?error=cluster" {
		t.Errorf("unreachable API server: %d to %q, want /login?error=cluster", rec.Code, rec.Header().Get("Location"))
	}
	// A token is read only from the form body, never from the URL.
	req := httptest.NewRequest("POST", "/auth/token?token="+url.QueryEscape(token), nil)
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if sessionOf(rec) != nil {
		t.Error("a token in the URL signed in")
	}
}

// logSink collects every log line written through the request's logger.
type logSink struct {
	mu    sync.Mutex
	lines []string
}

func (l *logSink) ctx() context.Context {
	return ctrllog.IntoContext(context.Background(), funcr.New(func(prefix, args string) {
		l.mu.Lock()
		defer l.mu.Unlock()
		l.lines = append(l.lines, prefix+" "+args)
	}, funcr.Options{Verbosity: 10}))
}

func (l *logSink) all() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return strings.Join(l.lines, "\n")
}

// TestTokenNeverLeaves fails if the token appears in a log line, a response
// body, a header or a URL during sign-in, reads or sign-out, or if the
// session outlives min(exp, 8h).
func TestTokenNeverLeaves(t *testing.T) {
	s, a := tokenServer(t, env.Config)
	start := time.Now().Truncate(time.Second)
	a.now = func() time.Time { return start }
	logs := &logSink{}
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.Handler().ServeHTTP(w, r.WithContext(logs.ctx()))
	})
	token := mintToken(t, "a", "leak-check", time.Hour)
	// The signature is the part that makes the token a credential.
	signature := token[strings.LastIndex(token, ".")+1:]

	var seen bytes.Buffer
	record := func(rec *httptest.ResponseRecorder) {
		seen.WriteString(rec.Body.String())
		for name, values := range rec.Header() {
			if name == "Set-Cookie" {
				continue // sealed; checked separately below
			}
			seen.WriteString(name + ": " + strings.Join(values, ",") + "\n")
		}
	}
	// A refused variant of the token logs its refusal.
	record(postToken(h, token+"x", logs.ctx()))
	signin := postToken(h, token, logs.ctx())
	record(signin)
	cookie := sessionOf(signin)
	if cookie == nil {
		t.Fatalf("sign-in failed: %d %s", signin.Code, signin.Header().Get("Location"))
	}
	if strings.Contains(cookie.Value, signature) {
		t.Fatal("the session cookie holds the token in clear")
	}
	for _, path := range []string{"/api/me", "/api/applications?namespace=a", "/api/applications?namespace=b", "/api/namespaces", "/api/applications/a/web"} {
		record(getWith(h, path, cookie))
	}
	out := httptest.NewRequest("POST", "/logout", nil)
	out.Header.Set("Sec-Fetch-Site", "same-origin")
	out.AddCookie(cookie)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, out)
	record(rec)

	if logs.all() == "" {
		t.Fatal("no log lines were captured, so the check proves nothing")
	}
	for where, text := range map[string]string{"a log line": logs.all(), "a response": seen.String()} {
		if strings.Contains(text, signature) {
			t.Errorf("the token appeared in %s:\n%s", where, text)
		}
	}

	// A one-hour token's session ends at its exp; a day-long token's after
	// 8 hours; and neither survives its end.
	id, err := a.Identity(requestWith(cookie))
	if err != nil {
		t.Fatal(err)
	}
	if want := start.Add(time.Hour); id.Expiry.Before(want.Add(-time.Minute)) || id.Expiry.After(want.Add(time.Minute)) {
		t.Errorf("one-hour token: session expires %v, want its exp near %v", id.Expiry, want)
	}
	if !cookie.Expires.Equal(id.Expiry) {
		t.Errorf("cookie expires %v, session %v", cookie.Expires, id.Expiry)
	}
	a.now = func() time.Time { return id.Expiry }
	if _, err := a.Identity(requestWith(cookie)); err == nil {
		t.Error("the session outlived the token's exp")
	}
	a.now = func() time.Time { return start }
	long := sessionOf(postToken(s.Handler(), mintToken(t, "a", "leak-check", 24*time.Hour), context.Background()))
	if long == nil {
		t.Fatal("day-long token sign-in failed")
	}
	id, err = a.Identity(requestWith(long))
	if err != nil {
		t.Fatal(err)
	}
	if want := start.Add(8 * time.Hour); !id.Expiry.Equal(want) {
		t.Errorf("day-long token: session expires %v, want %v", id.Expiry, want)
	}
	a.now = func() time.Time { return start.Add(8 * time.Hour) }
	if _, err := a.Identity(requestWith(long)); err == nil {
		t.Error("the session outlived 8 hours")
	}
	// Printing or encoding a session's Identity never shows the token.
	for _, text := range []string{fmt.Sprintf("%v %+v %#v %s", id, id, id, id.Token), mustJSON(t, id)} {
		if strings.Contains(text, signature) {
			t.Errorf("an Identity printed its token: %s", text)
		}
	}
}

func requestWith(c *http.Cookie) *http.Request {
	r := httptest.NewRequest("GET", "/api/me", nil)
	r.AddCookie(c)
	return r
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestTokenSessionCookieFitsInABrowser fails if a real ServiceAccount token
// with the longest names Kubernetes allows does not fit in one cookie, or if
// a token too large for one is accepted.
func TestTokenSessionCookieFitsInABrowser(t *testing.T) {
	ns := "n" + strings.Repeat("s", 62)    // the longest namespace name
	name := "s" + strings.Repeat("a", 252) // the longest ServiceAccount name
	s, _ := tokenServer(t, env.Config)
	rec := postToken(s.Handler(), mintToken(t, ns, name, time.Hour), context.Background())
	c := sessionOf(rec)
	if c == nil {
		t.Fatalf("longest-name token: %d to %q", rec.Code, rec.Header().Get("Location"))
	}
	if n := len(c.String()); n >= 4000 {
		t.Fatalf("the session cookie is %d bytes; browsers keep at most 4096 per cookie", n)
	}
	t.Logf("session cookie for a %d-byte token: %d bytes", len(mintToken(t, ns, name, time.Hour)), len(c.String()))

	api, _ := fakeReviewServer(t, http.StatusOK, authenticationv1.UserInfo{Username: "mia@acme.io"})
	fake, _ := tokenServer(t, &rest.Config{Host: api.URL})
	rec = postToken(fake.Handler(), strings.Repeat("t", 3500), context.Background())
	if rec.Header().Get("Location") != "/login?error=token" || sessionOf(rec) != nil {
		t.Fatalf("a token too large for a cookie: %d to %q", rec.Code, rec.Header().Get("Location"))
	}
}

// TestTokenSignInRefusesCrossSiteRequests fails if another site can post a
// token to sign a visitor in (login CSRF).
func TestTokenSignInRefusesCrossSiteRequests(t *testing.T) {
	api, calls := fakeReviewServer(t, http.StatusOK, authenticationv1.UserInfo{Username: "mia@acme.io"})
	s, _ := tokenServer(t, &rest.Config{Host: api.URL})
	for name, header := range map[string][2]string{
		"Sec-Fetch-Site": {"Sec-Fetch-Site", "cross-site"},
		"Origin":         {"Origin", "https://evil.example"},
	} {
		req := httptest.NewRequest("POST", "http://console.example/auth/token", strings.NewReader("token=attacker-token"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set(header[0], header[1])
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden || sessionOf(rec) != nil {
			t.Errorf("%s: cross-site sign-in = %d, cookie %v", name, rec.Code, sessionOf(rec) != nil)
		}
	}
	if calls.Load() != 0 {
		t.Errorf("a cross-site sign-in reached the API server")
	}
}

// A cookie from 0.4.x has no method and is an OIDC session; each method's
// cookie is judged by that method's rules.
func TestOIDCCookieWithoutMethodStillWorks(t *testing.T) {
	s, a := tokenServer(t, &restConfigForTest)
	exp := time.Now().Add(time.Hour).Truncate(time.Second)
	cookieOf := func(v any) *http.Cookie {
		sealed, err := seal(a.key, v)
		if err != nil {
			t.Fatal(err)
		}
		return &http.Cookie{Name: SessionCookie, Value: sealed}
	}
	legacy := cookieOf(map[string]any{"u": "mia@acme.io", "g": []string{"acme:platform"}, "e": exp})
	me := getWith(s.Handler(), "/api/me", legacy).Body.String()
	if !strings.Contains(me, `"username":"mia@acme.io"`) || !strings.Contains(me, `"method":"oidc"`) {
		t.Errorf("0.4.x cookie: /api/me = %s", me)
	}
	for name, tc := range map[string]struct {
		session map[string]any
		ok      bool
	}{
		"ServiceAccount token":    {map[string]any{"u": "system:serviceaccount:a:viewer", "g": []string{"system:serviceaccounts"}, "e": exp, "m": "token", "t": "tok"}, true},
		"ServiceAccount via OIDC": {map[string]any{"u": "system:serviceaccount:a:viewer", "e": exp, "m": "oidc"}, false},
		"legacy system: user":     {map[string]any{"u": "system:admin", "e": exp}, false},
		"token without a token":   {map[string]any{"u": "system:serviceaccount:a:viewer", "e": exp, "m": "token"}, false},
		"anonymous token":         {map[string]any{"u": "system:anonymous", "e": exp, "m": "token", "t": "tok"}, false},
		"unknown method":          {map[string]any{"u": "mia@acme.io", "e": exp, "m": "password", "t": "tok"}, false},
	} {
		_, err := a.Identity(requestWith(cookieOf(tc.session)))
		if (err == nil) != tc.ok {
			t.Errorf("%s: Identity err = %v, want ok=%v", name, err, tc.ok)
		}
	}
}
