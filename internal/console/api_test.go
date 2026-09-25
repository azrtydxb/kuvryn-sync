package console

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	corev1alpha1 "github.com/azrtydxb/kuvryn-sync/api/v1alpha1"
)

// env is an API server with RBAC authorization, shared by the API tests.
var env *envtest.Environment

func TestMain(m *testing.M) {
	env = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
		BinaryAssetsDirectory: envtestBinaryDir(),
	}
	env.ControlPlane.GetAPIServer().Configure().Set("authorization-mode", "RBAC")
	if _, err := env.Start(); err != nil {
		fmt.Fprintln(os.Stderr, "start envtest (run make setup-envtest):", err)
		os.Exit(1)
	}
	if err := seed(context.Background()); err != nil {
		_ = env.Stop()
		fmt.Fprintln(os.Stderr, "seed envtest:", err)
		os.Exit(1)
	}
	code := m.Run()
	_ = env.Stop()
	os.Exit(code)
}

// envtestBinaryDir finds the binaries make setup-envtest installs, so the
// tests also run without KUBEBUILDER_ASSETS.
func envtestBinaryDir() string {
	if os.Getenv("KUBEBUILDER_ASSETS") != "" {
		return ""
	}
	base := filepath.Join("..", "..", "bin", "k8s")
	entries, err := os.ReadDir(base)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if e.IsDir() {
			return filepath.Join(base, e.Name())
		}
	}
	return ""
}

func testScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(s)
	_ = corev1alpha1.AddToScheme(s)
	return s
}

// seed creates Applications web in a and db in b, a Secret a/creds labelled
// as managed by web, and binds alice to read Kuvryn Sync objects in a only.
func seed(ctx context.Context) error {
	c, err := client.New(env.Config, client.Options{Scheme: testScheme()})
	if err != nil {
		return err
	}
	managed := map[string]string{"sync.kuvryn.io/application": "web"}
	app := func(ns, name string) *corev1alpha1.Application {
		return &corev1alpha1.Application{
			ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: name},
			Spec: corev1alpha1.ApplicationSpec{Source: corev1alpha1.ApplicationSource{
				RepositoryRef: corev1alpha1.LocalObjectReference{Name: "platform"},
				Path:          "apps/" + name,
				Render:        corev1alpha1.RenderSpec{Type: corev1alpha1.RenderTypeYAML},
			}},
		}
	}
	web := app("a", "web")
	objs := []client.Object{
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "a"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "b"}},
		web,
		app("b", "db"),
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: "a", Name: "creds", Labels: managed}, Data: map[string][]byte{"password": []byte("secret")}},
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: "a", Name: "web-config", Labels: managed}, Data: map[string]string{"k": "v"}},
		&corev1alpha1.Repository{
			ObjectMeta: metav1.ObjectMeta{Namespace: "a", Name: "platform"},
			Spec: corev1alpha1.RepositorySpec{Type: corev1alpha1.RepositoryTypeGit, Git: &corev1alpha1.GitRepositorySpec{
				URL:  "https://git.example/platform.git",
				Auth: &corev1alpha1.GitAuthSpec{SecretRef: &corev1alpha1.SecretReference{Name: "creds"}},
			}},
		},
		&corev1alpha1.Revision{
			ObjectMeta: metav1.ObjectMeta{Namespace: "a", Name: "web-1", Labels: managed},
			Spec: corev1alpha1.RevisionSpec{
				ApplicationRef: corev1alpha1.LocalObjectReference{Name: "web"},
				Source: corev1alpha1.RevisionSource{
					RepositoryRef: corev1alpha1.LocalObjectReference{Name: "platform"},
					Revision:      "0123456789abcdef",
					Render:        corev1alpha1.RenderSpec{Type: corev1alpha1.RenderTypeYAML},
				},
			},
		},
		&corev1alpha1.ImagePolicy{
			ObjectMeta: metav1.ObjectMeta{Namespace: "a", Name: "web-image"},
			Spec: corev1alpha1.ImagePolicySpec{
				Image:     "registry.example/web",
				SecretRef: &corev1alpha1.SecretReference{Name: "creds"},
				Policy:    corev1alpha1.ImageSelectionPolicy{Semver: &corev1alpha1.SemverPolicy{Range: "1.x"}},
			},
		},
		&rbacv1.Role{
			ObjectMeta: metav1.ObjectMeta{Namespace: "a", Name: "kuvryn-sync-viewer"},
			Rules: []rbacv1.PolicyRule{{
				APIGroups: []string{"sync.kuvryn.io"},
				Resources: []string{"applications", "revisions", "repositories"},
				Verbs:     []string{"get", "list"},
			}},
		},
		&rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{Namespace: "a", Name: "alice"},
			RoleRef:    rbacv1.RoleRef{APIGroup: "rbac.authorization.k8s.io", Kind: "Role", Name: "kuvryn-sync-viewer"},
			Subjects:   []rbacv1.Subject{{APIGroup: "rbac.authorization.k8s.io", Kind: "User", Name: "alice"}},
		},
	}
	for _, obj := range objs {
		if err := c.Create(ctx, obj); err != nil {
			return fmt.Errorf("create %T %s: %w", obj, obj.GetName(), err)
		}
	}
	web.Status.ManagedKinds = []corev1alpha1.ManagedKind{{APIVersion: "v1", Kind: "ConfigMap"}, {APIVersion: "v1", Kind: "Secret"}}
	web.Status.Sync.State = corev1alpha1.SyncStateAwaitingApproval
	web.Status.Health.State = corev1alpha1.HealthStateDegraded
	web.Status.DeployedRevision = "0123456789abcdef"
	web.Status.Diagnosis = []corev1alpha1.DiagnosisCause{{
		Resource: corev1alpha1.ResourceRef{APIVersion: "v1", Kind: "Pod", Namespace: "a", Name: "web-abc"},
		Reason:   "ImagePullBackOff",
		Message:  "pull https://user:hunter2@registry.example/web failed",
		Chain: []corev1alpha1.ResourceRef{
			{APIVersion: "apps/v1", Kind: "Deployment", Namespace: "a", Name: "web"},
			{APIVersion: "v1", Kind: "Pod", Namespace: "a", Name: "web-abc"},
		},
	}}
	if err := c.Status().Update(ctx, web); err != nil {
		return fmt.Errorf("update web status: %w", err)
	}
	rev := &corev1alpha1.Revision{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: "a", Name: "web-1"}, rev); err != nil {
		return err
	}
	rev.Status.Phase = corev1alpha1.RevisionPhaseAwaitingApproval
	rev.Status.Plan = corev1alpha1.RevisionPlan{
		Digest:  "sha256:plan",
		Summary: corev1alpha1.PlanSummary{Create: 1, Update: 1},
		Resources: []corev1alpha1.PlanResourceChange{
			{Resource: corev1alpha1.ResourceRef{APIVersion: "v1", Kind: "ConfigMap", Namespace: "a", Name: "web-config"}, Action: corev1alpha1.PlanActionUpdate,
				Changes: []corev1alpha1.PlanFieldChange{{Path: "data.k", Before: "v", After: "w"}, {Path: "metadata.labels.tier", Before: "v", After: "w"}}},
			{Resource: corev1alpha1.ResourceRef{APIVersion: "v1", Kind: "Secret", Namespace: "a", Name: "web-tls"}, Action: corev1alpha1.PlanActionCreate,
				Changes: []corev1alpha1.PlanFieldChange{{Path: "data.tls.key", Redacted: true}}},
		},
	}
	return c.Status().Update(ctx, rev)
}

type fakeAuth struct{ id Identity }

func (f fakeAuth) Identity(*http.Request) (Identity, error) {
	if f.id.Username == "" {
		return Identity{}, ErrNoSession
	}
	return f.id, nil
}

func newAPIServerAs(t *testing.T, e *envtest.Environment, id Identity) *Server {
	t.Helper()
	s, err := NewServer(Config{ClusterName: "test"}, e.Config)
	if err != nil {
		t.Fatal(err)
	}
	s.UseAuthenticator(fakeAuth{id: id})
	return s
}

func newAPIServer(t *testing.T, e *envtest.Environment) *Server {
	return newAPIServerAs(t, e, Identity{Username: "alice"})
}

func get(t *testing.T, srv *Server, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
	return rec
}

func TestConsoleFollowsUserRBAC(t *testing.T) {
	srv := newAPIServer(t, env) // Server with a fake Auth returning Identity{Username: "alice"}
	all := get(t, srv, "/api/applications")
	if all.Code != 403 || !strings.Contains(all.Body.String(), `"needNamespace":true`) {
		t.Fatalf("cluster-wide list for a namespaced user = %d %s", all.Code, all.Body)
	}
	a := get(t, srv, "/api/applications?namespace=a")
	if a.Code != 200 || !strings.Contains(a.Body.String(), `"name":"web"`) {
		t.Fatalf("namespace a = %d %s", a.Code, a.Body)
	}
	b := get(t, srv, "/api/applications?namespace=b")
	if b.Code != 403 || strings.Contains(b.Body.String(), "db") {
		t.Fatalf("namespace b leaked: %d %s", b.Code, b.Body)
	}
}

func TestConsoleNeverReturnsSecrets(t *testing.T) {
	srv := newAPIServer(t, env)
	for _, path := range []string{"/api/me", "/api/namespaces", "/api/applications?namespace=a", "/api/applications/a/web", "/api/applications/a/web/revisions", "/api/applications/a/web/resources", "/api/repositories?namespace=a", "/api/revisions?namespace=a", "/api/imagepolicies?namespace=a"} {
		body := get(t, srv, path).Body.String()
		if strings.Contains(body, "creds") || strings.Contains(body, "c2VjcmV0") {
			t.Fatalf("%s returned Secret data: %s", path, body)
		}
	}
}

// bindBob lets bob read Kuvryn Sync objects, ConfigMaps and even Secrets in a.
func bindBob(t *testing.T) {
	t.Helper()
	c, err := client.New(env.Config, client.Options{Scheme: testScheme()})
	if err != nil {
		t.Fatal(err)
	}
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{Namespace: "a", Name: "bob"},
		Rules: []rbacv1.PolicyRule{
			{APIGroups: []string{"sync.kuvryn.io"}, Resources: []string{"*"}, Verbs: []string{"get", "list"}},
			{APIGroups: []string{""}, Resources: []string{"configmaps", "secrets"}, Verbs: []string{"get", "list"}},
		},
	}
	binding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Namespace: "a", Name: "bob"},
		RoleRef:    rbacv1.RoleRef{APIGroup: "rbac.authorization.k8s.io", Kind: "Role", Name: "bob"},
		Subjects:   []rbacv1.Subject{{APIGroup: "rbac.authorization.k8s.io", Kind: "Group", Name: "team-b"}},
	}
	for _, obj := range []client.Object{role, binding} {
		if err := c.Create(context.Background(), obj); client.IgnoreAlreadyExists(err) != nil {
			t.Fatal(err)
		}
	}
}

func TestConsoleAPIShowsWhatTheUserMayRead(t *testing.T) {
	alice := newAPIServer(t, env)
	for path, want := range map[string][]string{
		"/api/applications/a/web": {
			`"sync":"AwaitingApproval"`, `"health":"Degraded"`, `"reason":"ImagePullBackOff"`,
			`{"kind":"Pod","name":"web-abc","state":"ImagePullBackOff"}`, `{"kind":"Deployment","name":"web","state":"—"}`,
			`"digest":"sha256:plan"`, `"planVisible":true`, `"path":"metadata.labels.tier","before":"v","after":"w","redacted":false`, `"path":"data.k","before":"REDACTED","after":"REDACTED","redacted":true`,
		},
		"/api/applications/a/web/revisions": {`"name":"web-1"`, `"phase":"AwaitingApproval"`, `"commit":"0123456789abcdef"`},
		"/api/revisions?namespace=a":        {`"application":"web"`, `"approvedBy":"—"`},
		"/api/repositories?namespace=a":     {`"url":"https://git.example/platform.git"`, `"apps":1`, `"state":"Unknown"`},
		"/api/applications/a/web/resources": {
			`{"kind":"ConfigMap","name":"—","apiVersion":"—","sync":"—","health":"—","visible":false}`,
			`{"kind":"Secret","name":"web-tls","apiVersion":"v1","sync":"—","health":"—","visible":false}`,
		},
	} {
		rec := get(t, alice, path)
		for _, w := range want {
			if rec.Code != 200 || !strings.Contains(rec.Body.String(), w) {
				t.Errorf("%s = %d, lacks %s:\n%s", path, rec.Code, w, rec.Body.String())
			}
		}
		if strings.Contains(rec.Body.String(), "hunter2") {
			t.Errorf("%s did not redact a credential: %s", path, rec.Body.String())
		}
	}
	detail := get(t, alice, "/api/applications/a/web").Body.String()
	if !strings.Contains(detail, `"path":"data.tls.key","before":"","after":"","redacted":true`) {
		t.Errorf("Secret change not redacted: %s", detail)
	}
	for path, code := range map[string]int{
		"/api/namespaces":                  403,
		"/api/imagepolicies?namespace=a":   403,
		"/api/imagepolicies":               403,
		"/api/applications/b/db":           403,
		"/api/applications/a/missing":      404,
		"/api/applications/b/db/resources": 403,
		"/api/applications/a/web/nonsense": 404,
		"/api/repositories?namespace=b":    403,
	} {
		if rec := get(t, alice, path); rec.Code != code {
			t.Errorf("%s = %d, want %d: %s", path, rec.Code, code, rec.Body.String())
		}
	}

	bindBob(t)
	bob := newAPIServerAs(t, env, Identity{Username: "bob", Groups: []string{"team-b"}})
	res := get(t, bob, "/api/applications/a/web/resources").Body.String()
	if !strings.Contains(res, `{"kind":"ConfigMap","name":"web-config","apiVersion":"v1","sync":"OutOfSync","health":"—","visible":true}`) ||
		!strings.Contains(res, `"name":"web-tls"`) || strings.Contains(res, "creds") {
		t.Fatalf("resources for a user who may read ConfigMaps and Secrets = %s", res)
	}
	if ip := get(t, bob, "/api/imagepolicies?namespace=a"); ip.Code != 200 || !strings.Contains(ip.Body.String(), `"rule":"semver 1.x"`) || strings.Contains(ip.Body.String(), "creds") {
		t.Fatalf("image policies = %d %s", ip.Code, ip.Body.String())
	}
}

func TestConsoleRefusesSystemIdentitiesAndAnonymous(t *testing.T) {
	for _, id := range []Identity{{}, {Username: "system:admin"}, {Username: "eve", Groups: []string{"system:masters"}}} {
		srv := newAPIServerAs(t, env, id)
		if rec := get(t, srv, "/api/applications?namespace=b"); rec.Code != http.StatusUnauthorized {
			t.Fatalf("%+v = %d %s", id, rec.Code, rec.Body.String())
		}
		if rec := get(t, srv, "/api/me"); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"authenticated":false`) || strings.Contains(rec.Body.String(), "system:") {
			t.Fatalf("%+v /api/me = %d %s", id, rec.Code, rec.Body.String())
		}
	}
}

type blockingReader struct{ client.Reader }

func (blockingReader) List(ctx context.Context, _ client.ObjectList, _ ...client.ListOption) error {
	<-ctx.Done()
	return ctx.Err()
}

func TestConsoleAPITimesOut(t *testing.T) {
	srv := newAPIServer(t, env)
	srv.newReader = func(Identity) (client.Reader, error) { return blockingReader{}, nil }
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/api/applications", nil).WithContext(ctx))
	if rec.Code != http.StatusGatewayTimeout || !strings.Contains(rec.Body.String(), `"error":"timeout"`) {
		t.Fatalf("slow API server = %d %s", rec.Code, rec.Body.String())
	}
}

func TestViewsShowUnknownForMissingStatus(t *testing.T) {
	row := appRow(&corev1alpha1.Application{ObjectMeta: metav1.ObjectMeta{Name: "new", Namespace: "a"}})
	if row.Sync != "Unknown" || row.Health != "Unknown" || row.Commit != "—" || row.LastReconcile != "—" || row.Repository != "—" || row.Destination != "a" {
		t.Fatalf("new Application row = %+v", row)
	}
	d := appDetail(&corev1alpha1.Application{}, nil, true)
	if d.Plan != nil || len(d.Diagnosis) != 0 || d.Diagnosis == nil || d.Conditions == nil {
		t.Fatalf("empty detail = %+v", d)
	}
}
