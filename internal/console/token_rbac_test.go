package console

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// boundToken returns a token for ServiceAccount ns/name that is valid only
// while the Secret ns/bound exists, so deleting the Secret revokes it alone.
func boundToken(t *testing.T, cs kubernetes.Interface, ns, name, bound string) string {
	t.Helper()
	ctx := context.Background()
	secret, err := cs.CoreV1().Secrets(ns).Create(ctx, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: bound}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	seconds := int64(3600)
	tr, err := cs.CoreV1().ServiceAccounts(ns).CreateToken(ctx, name, &authenticationv1.TokenRequest{Spec: authenticationv1.TokenRequestSpec{
		ExpirationSeconds: &seconds,
		BoundObjectRef:    &authenticationv1.BoundObjectReference{Kind: "Secret", APIVersion: "v1", Name: bound, UID: secret.UID},
	}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return tr.Status.Token
}

// TestTokenSessionFollowsRBAC fails if a token with no access to a namespace
// sees its Applications, if a token with access does not, or if one token's
// reader serves another token of the same identity.
func TestTokenSessionFollowsRBAC(t *testing.T) {
	cs, err := kubernetes.NewForConfig(env.Config)
	if err != nil {
		t.Fatal(err)
	}
	reader := mintToken(t, "a", "reader", time.Hour)
	nobody := mintToken(t, "a", "nobody", time.Hour)
	_, err = cs.RbacV1().RoleBindings("a").Create(context.Background(), &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "reader"},
		RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "Role", Name: "kuvryn-sync-viewer"},
		Subjects:   []rbacv1.Subject{{Kind: "ServiceAccount", Namespace: "a", Name: "reader"}},
	}, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatal(err)
	}
	s, _ := tokenServer(t, env.Config)
	h := s.Handler()
	signIn := func(token string) *http.Cookie {
		t.Helper()
		rec := postToken(h, token, context.Background())
		c := sessionOf(rec)
		if c == nil {
			t.Fatalf("sign-in failed: %d to %q", rec.Code, rec.Header().Get("Location"))
		}
		return c
	}
	readerSession, nobodySession := signIn(reader), signIn(nobody)
	if rec := getWith(h, "/api/applications?namespace=a", readerSession); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"name":"web"`) {
		t.Errorf("reader in a = %d %s, want web", rec.Code, rec.Body)
	}
	if rec := getWith(h, "/api/applications?namespace=b", readerSession); rec.Code != 403 || strings.Contains(rec.Body.String(), "db") {
		t.Errorf("reader in b = %d %s, want 403", rec.Code, rec.Body)
	}
	if rec := getWith(h, "/api/applications?namespace=a", nobodySession); rec.Code != 403 || strings.Contains(rec.Body.String(), "web") {
		t.Errorf("nobody in a = %d %s, want 403", rec.Code, rec.Body)
	}
	if rec := getWith(h, "/api/applications/a/web", nobodySession); rec.Code != 403 {
		t.Errorf("nobody reading a/web = %d %s, want 403", rec.Code, rec.Body)
	}

	// Two tokens for the same ServiceAccount: revoking the first must not
	// break the second, and the first's session must end.
	first := signIn(boundToken(t, cs, "a", "reader", "reader-first"))
	second := signIn(boundToken(t, cs, "a", "reader", "reader-second"))
	if rec := getWith(h, "/api/applications?namespace=a", first); rec.Code != 200 {
		t.Fatalf("first token before revocation = %d %s", rec.Code, rec.Body)
	}
	if err := cs.CoreV1().Secrets("a").Delete(context.Background(), "reader-first", metav1.DeleteOptions{}); err != nil {
		t.Fatal(err)
	}
	// The API server caches a token's authentication for 10 seconds, so the
	// revocation takes effect within that; wait for it with room to spare.
	deadline := time.Now().Add(30 * time.Second)
	var revoked *httptestResponse
	for time.Now().Before(deadline) {
		rec := getWith(h, "/api/applications?namespace=a", first)
		if rec.Code == http.StatusUnauthorized {
			revoked = &httptestResponse{code: rec.Code, cookies: rec.Result().Cookies()}
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if revoked == nil {
		t.Fatal("the revoked token's session still reads")
	}
	cleared := false
	for _, c := range revoked.cookies {
		if c.Name == SessionCookie && c.Value == "" && c.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Errorf("a 401 from the API server did not clear the session cookie: %+v", revoked.cookies)
	}
	if rec := getWith(h, "/api/applications?namespace=a", second); rec.Code != 200 {
		t.Errorf("second token after the first was revoked = %d %s; a reader was shared between tokens", rec.Code, rec.Body)
	}
}

type httptestResponse struct {
	code    int
	cookies []*http.Cookie
}

// TestReaderCacheSeparatesTokens fails if two tokens of one identity share a
// reader, or if the cache key holds the token.
func TestReaderCacheSeparatesTokens(t *testing.T) {
	builds := 0
	c := newReaderCache(func(Identity) (client.Reader, error) {
		builds++
		return nil, nil
	})
	exp := time.Now().Add(time.Hour)
	one := Identity{Username: "system:serviceaccount:a:reader", Groups: []string{"system:serviceaccounts"}, Expiry: exp, Method: MethodToken, Token: "token-one"}
	two := one
	two.Token = "token-two"
	oidc := Identity{Username: one.Username, Groups: one.Groups, Expiry: exp}
	for _, id := range []Identity{one, two, one, two, oidc} {
		if _, err := c.get(id); err != nil {
			t.Fatal(err)
		}
	}
	if builds != 3 {
		t.Fatalf("%d builds for two tokens and one OIDC session of one identity, want 3", builds)
	}
	for _, id := range []Identity{one, two} {
		if key := readerKey(id); strings.Contains(key, id.Token.Reveal()) {
			t.Fatalf("reader key %q holds the token", key)
		}
	}
}

// A read refused for carrying the wrong credential is Forbidden, not a
// cluster failure, and no error the console logs holds the session's token.
func TestReadErrorsNeverLogTheToken(t *testing.T) {
	s, err := NewServer(Config{ClusterName: "test"}, &restConfigForTest)
	if err != nil {
		t.Fatal(err)
	}
	logs := &logSink{}
	id := Identity{Username: "system:serviceaccount:a:viewer", Expiry: time.Now().Add(time.Hour), Method: MethodToken, Token: "secret-session-token"}
	for err, want := range map[error]int{
		ErrNotTheSessionToken: http.StatusForbidden,
		errors.New(`Get "https://k8s/api": dial tcp: secret-session-token refused`): http.StatusBadGateway,
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/api/applications", nil).WithContext(logs.ctx())
		s.writeError(req.Context(), rec, req, id, err)
		if rec.Code != want {
			t.Errorf("%v: status %d, want %d", err, rec.Code, want)
		}
	}
	if strings.Contains(logs.all(), "secret-session-token") {
		t.Fatalf("a read error logged the token: %s", logs.all())
	}
}
