package console

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// clientCertPEM returns a self-signed client certificate and key, standing in
// for the console's own client certificate credentials.
func clientCertPEM(t *testing.T) (certPEM, keyPEM []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "system:serviceaccount:kuvryn-sync-system:console"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
}

// TestTokenSessionReadsAsTheToken fails if a token session's request carries
// any of the console's own credentials or an Impersonate-* header, or if a
// write, a Secret read or a subresource request leaves the process.
func TestTokenSessionReadsAsTheToken(t *testing.T) {
	var mu sync.Mutex
	var seen []*http.Request
	api := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.Clone(context.Background()))
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api":
			_, _ = io.WriteString(w, `{"kind":"APIVersions","versions":["v1"]}`)
		case "/apis":
			_, _ = io.WriteString(w, `{"kind":"APIGroupList","groups":[]}`)
		case "/api/v1":
			_, _ = io.WriteString(w, `{"kind":"APIResourceList","groupVersion":"v1","resources":[`+
				`{"name":"configmaps","namespaced":true,"kind":"ConfigMap","verbs":["get","list","create"]},`+
				`{"name":"pods","namespaced":true,"kind":"Pod","verbs":["get","list"]},`+
				`{"name":"pods/log","namespaced":true,"kind":"Pod","verbs":["get"]},`+
				`{"name":"secrets","namespaced":true,"kind":"Secret","verbs":["get","list"]}]}`)
		default:
			_, _ = io.WriteString(w, `{"kind":"ConfigMap","apiVersion":"v1","metadata":{"name":"x","namespace":"a"}}`)
		}
	}))
	api.TLS = &tls.Config{ClientAuth: tls.RequestClientCert}
	api.StartTLS()
	defer api.Close()

	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: api.Certificate().Raw})
	certPEM, keyPEM := clientCertPEM(t)
	tokenFile := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(tokenFile, []byte("console-token-from-file"), 0o600); err != nil {
		t.Fatal(err)
	}
	// The console's own configuration, with every kind of credential set.
	base := &rest.Config{
		Host:            api.URL,
		BearerToken:     "console-token",
		BearerTokenFile: tokenFile,
		Username:        "console",
		Password:        "console-password",
		Impersonate:     rest.ImpersonationConfig{UserName: "admin", Groups: []string{"system:masters"}, UID: "1"},
		TLSClientConfig: rest.TLSClientConfig{CAData: caPEM, CertData: certPEM, KeyData: keyPEM},
		ExecProvider:    &clientcmdapi.ExecConfig{Command: "/nonexistent/credential-plugin", APIVersion: "client.authentication.k8s.io/v1", InteractiveMode: clientcmdapi.NeverExecInteractiveMode},
		AuthProvider:    &clientcmdapi.AuthProviderConfig{Name: "oidc"},
		WrapTransport: func(rt http.RoundTripper) http.RoundTripper {
			return roundTripFunc(func(r *http.Request) (*http.Response, error) {
				r.Header.Set("Impersonate-User", "admin")
				r.Header.Set("Authorization", "Bearer console-token")
				return rt.RoundTrip(r)
			})
		},
	}
	id := Identity{Username: "system:serviceaccount:a:viewer", Groups: []string{"system:serviceaccounts", "system:authenticated"},
		Expiry: time.Now().Add(time.Hour), Method: MethodToken, Token: "user-token"}
	reader, err := TokenClient(base, scheme.Scheme, id)
	if err != nil {
		t.Fatalf("TokenClient: %v", err)
	}
	ctx := context.Background()
	if err := reader.Get(ctx, client.ObjectKey{Namespace: "a", Name: "x"}, &corev1.ConfigMap{}); err != nil {
		t.Fatalf("ConfigMap read: %v", err)
	}
	if err := reader.List(ctx, &corev1.ConfigMapList{}, client.InNamespace("a")); err != nil {
		t.Fatalf("ConfigMap list: %v", err)
	}
	mu.Lock()
	reads := len(seen)
	mu.Unlock()

	// Nothing but GETs of allowed paths may leave the process.
	full, ok := reader.(client.Client)
	if !ok {
		t.Fatalf("TokenClient returned %T, which cannot be probed for writes", reader)
	}
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: "a", Name: "new"}}
	if err := full.Create(ctx, cm); !errors.Is(err, ErrWriteRefused) {
		t.Errorf("Create = %v, want ErrWriteRefused", err)
	}
	if err := full.Delete(ctx, cm); !errors.Is(err, ErrWriteRefused) {
		t.Errorf("Delete = %v, want ErrWriteRefused", err)
	}
	if err := reader.Get(ctx, client.ObjectKey{Namespace: "a", Name: "creds"}, &corev1.Secret{}); !errors.Is(err, ErrForbiddenPath) {
		t.Errorf("Secret read = %v, want ErrForbiddenPath", err)
	}
	if err := reader.List(ctx, &corev1.SecretList{}); !errors.Is(err, ErrForbiddenPath) {
		t.Errorf("Secret list = %v, want ErrForbiddenPath", err)
	}
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "a", Name: "web"}}
	if err := full.SubResource("log").Get(ctx, pod, &corev1.Pod{}); !errors.Is(err, ErrForbiddenPath) {
		t.Errorf("pods/log = %v, want ErrForbiddenPath", err)
	}
	if err := full.SubResource("status").Get(ctx, pod, &corev1.Pod{}); !errors.Is(err, ErrForbiddenPath) {
		t.Errorf("pods/status = %v, want ErrForbiddenPath", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if reads == 0 {
		t.Fatal("no request reached the API server")
	}
	if len(seen) != reads {
		for _, r := range seen[reads:] {
			t.Errorf("refused request reached the API server: %s %s", r.Method, r.URL.Path)
		}
	}
	for _, r := range seen {
		desc := r.Method + " " + r.URL.Path
		if r.Method != http.MethodGet {
			t.Errorf("%s: not a GET", desc)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer user-token" {
			t.Errorf("%s: Authorization = %q, want the session's token only", desc, got)
		}
		for name := range r.Header {
			if strings.HasPrefix(strings.ToLower(name), "impersonate-") {
				t.Errorf("%s: carries %s", desc, name)
			}
		}
		if r.TLS == nil || len(r.TLS.PeerCertificates) != 0 {
			t.Errorf("%s: presented a client certificate", desc)
		}
	}
}

// The transport itself refuses, in token mode, anything that is not the
// session's own token: an impersonation header of any kind, another bearer
// token, basic auth, or no credential at all.
func TestTokenTransportRefusesOtherCredentials(t *testing.T) {
	var sent int
	rt := roundTripFunc(func(*http.Request) (*http.Response, error) {
		sent++
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
	})
	tr := readOnlyTransport{next: rt, token: "user-token"}
	req := func(header map[string]string) *http.Request {
		r, _ := http.NewRequest("GET", "https://k8s/api/v1/namespaces/a/configmaps", nil)
		for k, v := range header {
			r.Header.Set(k, v)
		}
		return r
	}
	for name, h := range map[string]map[string]string{
		"Impersonate-User":        {"Authorization": "Bearer user-token", "Impersonate-User": "alice"},
		"Impersonate-Group":       {"Authorization": "Bearer user-token", "Impersonate-Group": "team-a"},
		"Impersonate-Uid":         {"Authorization": "Bearer user-token", "Impersonate-Uid": "1"},
		"Impersonate-Extra-scope": {"Authorization": "Bearer user-token", "Impersonate-Extra-Scope": "x"},
		"another token":           {"Authorization": "Bearer console-token"},
		"basic auth":              {"Authorization": "Basic Y29uc29sZTpwdw=="},
		"no credential":           {},
	} {
		if _, err := tr.RoundTrip(req(h)); !errors.Is(err, ErrNotTheSessionToken) {
			t.Errorf("%s: err = %v, want ErrNotTheSessionToken", name, err)
		}
	}
	if sent != 0 {
		t.Fatalf("%d refused requests were sent", sent)
	}
	if _, err := tr.RoundTrip(req(map[string]string{"Authorization": "Bearer user-token"})); err != nil || sent != 1 {
		t.Fatalf("the session's own token was refused: %v", err)
	}
	post, _ := http.NewRequest("POST", "https://k8s/api/v1/namespaces/a/configmaps", nil)
	post.Header.Set("Authorization", "Bearer user-token")
	if _, err := tr.RoundTrip(post); !errors.Is(err, ErrWriteRefused) {
		t.Fatalf("POST in token mode = %v, want ErrWriteRefused", err)
	}
	if _, err := TokenClient(&rest.Config{Host: "https://k8s"}, scheme.Scheme, Identity{Username: "u", Method: MethodToken, Expiry: time.Now().Add(time.Hour)}); err == nil {
		t.Fatal("TokenClient accepted a session without a token")
	}
}
