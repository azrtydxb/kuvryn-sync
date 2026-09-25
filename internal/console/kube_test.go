package console

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestConsoleClientIsReadOnly(t *testing.T) {
	var sent []string
	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		sent = append(sent, r.Method+" "+r.URL.Path)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{}`)), Header: http.Header{"Content-Type": {"application/json"}}}, nil
	})
	for _, method := range []string{"POST", "PUT", "PATCH", "DELETE"} {
		req, _ := http.NewRequest(method, "https://k8s/apis/sync.kuvryn.io/v1alpha1/namespaces/a/applications/x", nil)
		if _, err := (readOnlyTransport{next: rt}).RoundTrip(req); !errors.Is(err, ErrWriteRefused) {
			t.Fatalf("%s allowed: %v", method, err)
		}
	}
	req, _ := http.NewRequest("GET", "https://k8s/api/v1/namespaces/a/secrets/db", nil)
	if _, err := (readOnlyTransport{next: rt}).RoundTrip(req); err == nil {
		t.Fatal("Secret read allowed")
	}
	if len(sent) != 0 {
		t.Fatalf("requests reached the API server: %v", sent)
	}
	if _, err := UserClient(&rest.Config{Host: "https://k8s"}, scheme.Scheme, Identity{Username: "system:admin"}); err == nil {
		t.Fatal("system: user admitted")
	}
	if _, err := UserClient(&rest.Config{Host: "https://k8s"}, scheme.Scheme, Identity{Username: "a", Groups: []string{"system:masters"}}); err == nil {
		t.Fatal("system: group admitted")
	}
}

func TestUserClientSendsOnlyImpersonatedGETs(t *testing.T) {
	var mu sync.Mutex
	var seen []string
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.Method+" "+r.URL.Path+" as "+r.Header.Get("Impersonate-User")+" "+strings.Join(r.Header.Values("Impersonate-Group"), ","))
		mu.Unlock()
		switch r.URL.Path {
		case "/api":
			_, _ = io.WriteString(w, `{"kind":"APIVersions","versions":["v1"]}`)
		case "/apis":
			_, _ = io.WriteString(w, `{"kind":"APIGroupList","groups":[]}`)
		case "/api/v1":
			_, _ = io.WriteString(w, `{"kind":"APIResourceList","groupVersion":"v1","resources":[`+
				`{"name":"configmaps","namespaced":true,"kind":"ConfigMap","verbs":["get","list"]},`+
				`{"name":"secrets","namespaced":true,"kind":"Secret","verbs":["get","list"]}]}`)
		default:
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"kind":"ConfigMap","apiVersion":"v1","metadata":{"name":"x","namespace":"a"}}`)
		}
	}))
	defer api.Close()
	c, err := UserClient(&rest.Config{Host: api.URL}, scheme.Scheme, Identity{Username: "alice", Groups: []string{"team-a"}})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := c.Get(ctx, client.ObjectKey{Namespace: "a", Name: "x"}, &corev1.ConfigMap{}); err != nil {
		t.Fatalf("ConfigMap read: %v", err)
	}
	if err := c.Get(ctx, client.ObjectKey{Namespace: "a", Name: "creds"}, &corev1.Secret{}); !errors.Is(err, ErrForbiddenPath) {
		t.Fatalf("Secret read = %v, want ErrForbiddenPath", err)
	}
	if err := c.List(ctx, &corev1.SecretList{}); !errors.Is(err, ErrForbiddenPath) {
		t.Fatalf("Secret list = %v, want ErrForbiddenPath", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(seen) == 0 {
		t.Fatal("no request reached the API server")
	}
	for _, s := range seen {
		if !strings.HasPrefix(s, "GET ") || !strings.HasSuffix(s, " as alice team-a") || strings.Contains(s, "secrets") {
			t.Fatalf("request %q was not an impersonated, non-Secret GET (all: %v)", s, seen)
		}
	}
}

func TestForbiddenPaths(t *testing.T) {
	for path, want := range map[string]bool{
		// Secrets, however the path is spelled.
		"/api/v1/secrets":                                 true,
		"/api/v1/namespaces/a/secrets":                    true,
		"/api/v1/namespaces/a/secrets/db":                 true,
		"/api/v1/watch/secrets":                           true,
		"/api/v1/watch/namespaces/a/secrets":              true,
		"/api/v1//namespaces/a/secrets":                   true,
		"//api/v1/namespaces/a//secrets/db":               true,
		"/api/v1/namespaces/a/./secrets":                  true,
		"/api/v1/namespaces/a/configmaps/../../a/secrets": true,
		// Subresources, proxies, watches and anything outside the resource API.
		"/api/v1/namespaces/a/pods/p/exec":               true,
		"/api/v1/namespaces/a/pods/p/log":                true,
		"/api/v1/namespaces/a/services/s/proxy/x":        true,
		"/api/v1/nodes/n/proxy":                          true,
		"/api/v1/proxy/nodes/n":                          true,
		"/apis/apps/v1/namespaces/a/deployments/x/scale": true,
		"/apis/apps/v1/watch/deployments":                true,
		"/version":                                       true,
		"/openapi/v3":                                    true,
		"/logs/":                                         true,
		"/":                                              true,
		// What the console reads: discovery, lists and gets.
		"/api":                            false,
		"/api/v1":                         false,
		"/apis":                           false,
		"/apis/apps/v1":                   false,
		"/api/v1/namespaces":              false,
		"/api/v1/namespaces/a":            false,
		"/api/v1/namespaces/a/configmaps": false,
		"/apis/apps/v1/namespaces/a/deployments/proxy":              false,
		"/apis/sync.kuvryn.io/v1alpha1/namespaces/a/applications":   false,
		"/apis/sync.kuvryn.io/v1alpha1/namespaces/a/applications/x": false,
		"/apis/example.io/v1/namespaces/a/secrets":                  false,
	} {
		if got := forbiddenPath(path); got != want {
			t.Errorf("forbiddenPath(%q) = %v, want %v", path, got, want)
		}
	}
	watch, _ := http.NewRequest("GET", "https://k8s/api/v1/namespaces/a/configmaps?watch=true", nil)
	watch.Header.Set("Impersonate-User", "alice")
	if _, err := (readOnlyTransport{next: http.DefaultTransport, user: "alice"}).RoundTrip(watch); !errors.Is(err, ErrForbiddenPath) {
		t.Fatalf("watch = %v, want ErrForbiddenPath", err)
	}
	req, _ := http.NewRequest("GET", "https://k8s/api/v1/namespaces/a/configmaps", nil)
	if _, err := (readOnlyTransport{next: http.DefaultTransport}).RoundTrip(req); !errors.Is(err, ErrNotImpersonated) {
		t.Fatalf("unimpersonated GET = %v, want ErrNotImpersonated", err)
	}
	req.Header.Set("Impersonate-User", "bob")
	if _, err := (readOnlyTransport{next: http.DefaultTransport, user: "alice"}).RoundTrip(req); !errors.Is(err, ErrNotImpersonated) {
		t.Fatalf("GET as another user = %v, want ErrNotImpersonated", err)
	}
	req.Header.Set("Impersonate-User", "alice")
	req.Header.Set("Upgrade", "websocket")
	if _, err := (readOnlyTransport{next: http.DefaultTransport, user: "alice"}).RoundTrip(req); !errors.Is(err, ErrForbiddenPath) {
		t.Fatalf("upgrade = %v, want ErrForbiddenPath", err)
	}
}
