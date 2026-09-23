package receiver

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"golang.org/x/time/rate"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	corev1alpha1 "github.com/azrtydxb/solder/api/v1alpha1"
)

const githubPush = `{"ref":"refs/heads/main","repository":{"clone_url":"https://github.com/acme/platform.git"}}`

func newReceiver(t *testing.T, limit rate.Limit, burst int) (*Receiver, client.Client) {
	t.Helper()
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = corev1alpha1.AddToScheme(scheme)
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		&corev1alpha1.Repository{
			ObjectMeta: metav1.ObjectMeta{Name: "platform", Namespace: "default"},
			Spec: corev1alpha1.RepositorySpec{
				Git:     &corev1alpha1.GitRepositorySpec{URL: "git@github.com:acme/platform.git"},
				Webhook: &corev1alpha1.RepositoryWebhook{SecretRef: corev1alpha1.SecretReference{Name: "hook"}},
			},
		},
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "hook", Namespace: "default"}, Data: map[string][]byte{"token": []byte("s3cret")}},
	).Build()
	return &Receiver{Client: c, Limit: limit, Burst: burst}, c
}

func sign(body string) string {
	mac := hmac.New(sha256.New, []byte("s3cret"))
	mac.Write([]byte(body))
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func post(r *Receiver, path, body string, headers map[string]string) int {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, req)
	return rec.Code
}

func requestedAt(t *testing.T, c client.Client) string {
	t.Helper()
	repo := &corev1alpha1.Repository{}
	if err := c.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "platform"}, repo); err != nil {
		t.Fatal(err)
	}
	return repo.GetAnnotations()[RequestedAtAnnotation]
}

func TestSignedGitHubPushRequestsAFetch(t *testing.T) {
	r, c := newReceiver(t, rate.Inf, 1)
	code := post(r, "/hooks/default/platform", githubPush, map[string]string{"X-GitHub-Event": "push", "X-Hub-Signature-256": sign(githubPush)})
	if code != http.StatusAccepted || requestedAt(t, c) == "" {
		t.Fatalf("code = %d, annotation = %q", code, requestedAt(t, c))
	}
}

func TestGitLabTokenPushRequestsAFetch(t *testing.T) {
	r, c := newReceiver(t, rate.Inf, 1)
	body := `{"project":{"git_ssh_url":"git@github.com:acme/platform.git"}}`
	code := post(r, "/hooks/default/platform", body, map[string]string{"X-Gitlab-Event": "Push Hook", "X-Gitlab-Token": "s3cret"})
	if code != http.StatusAccepted || requestedAt(t, c) == "" {
		t.Fatalf("code = %d", code)
	}
}

func TestReceiverRejects(t *testing.T) {
	cases := map[string]struct {
		path    string
		body    string
		headers map[string]string
		want    int
	}{
		"bad signature":     {"/hooks/default/platform", githubPush, map[string]string{"X-GitHub-Event": "push", "X-Hub-Signature-256": "sha256=00"}, http.StatusUnauthorized},
		"bad gitlab token":  {"/hooks/default/platform", githubPush, map[string]string{"X-Gitlab-Event": "Push Hook", "X-Gitlab-Token": "wrong"}, http.StatusUnauthorized},
		"no signature":      {"/hooks/default/platform", githubPush, map[string]string{"X-GitHub-Event": "push"}, http.StatusUnauthorized},
		"unknown repo":      {"/hooks/default/missing", githubPush, map[string]string{"X-GitHub-Event": "push", "X-Hub-Signature-256": sign(githubPush)}, http.StatusNotFound},
		"other repository":  {"/hooks/default/platform", `{"repository":{"clone_url":"https://github.com/evil/other.git"}}`, map[string]string{"X-GitHub-Event": "push", "X-Hub-Signature-256": sign(`{"repository":{"clone_url":"https://github.com/evil/other.git"}}`)}, http.StatusBadRequest},
		"oversized payload": {"/hooks/default/platform", strings.Repeat("x", maxBody+1), map[string]string{"X-GitHub-Event": "push"}, http.StatusRequestEntityTooLarge},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			r, c := newReceiver(t, rate.Inf, 1)
			if code := post(r, tc.path, tc.body, tc.headers); code != tc.want {
				t.Fatalf("code = %d, want %d", code, tc.want)
			}
			if requestedAt(t, c) != "" {
				t.Fatal("a rejected request requested a fetch")
			}
		})
	}
}

func TestReceiverRateLimitsPerRepository(t *testing.T) {
	r, _ := newReceiver(t, rate.Every(1<<62), 1)
	headers := map[string]string{"X-GitHub-Event": "push", "X-Hub-Signature-256": sign(githubPush)}
	if code := post(r, "/hooks/default/platform", githubPush, headers); code != http.StatusAccepted {
		t.Fatalf("first = %d", code)
	}
	if code := post(r, "/hooks/default/platform", githubPush, headers); code != http.StatusTooManyRequests {
		t.Fatalf("second = %d", code)
	}
}

func TestReceiverRunsOnEveryReplica(t *testing.T) {
	if (&Receiver{}).NeedLeaderElection() {
		t.Fatal("receiver would only listen on the leader, but the Service routes to every replica")
	}
}

func TestRegistryWebhookRequestsAnImageScan(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = corev1alpha1.AddToScheme(scheme)
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		&corev1alpha1.ImagePolicy{
			ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "payments"},
			Spec: corev1alpha1.ImagePolicySpec{
				Image:   "ghcr.io/acme/api",
				Policy:  corev1alpha1.ImageSelectionPolicy{Semver: &corev1alpha1.SemverPolicy{Range: "^1"}},
				Webhook: &corev1alpha1.ImagePolicyWebhook{SecretRef: corev1alpha1.SecretReference{Name: "registry-hook"}},
			},
		},
		&corev1alpha1.ImagePolicy{ObjectMeta: metav1.ObjectMeta{Name: "no-hook", Namespace: "payments"}},
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "registry-hook", Namespace: "payments"}, Data: map[string][]byte{"token": []byte("reg-token")}},
	).Build()
	r := &Receiver{Client: c, Limit: rate.Inf, Burst: 1}
	requested := func() string {
		policy := &corev1alpha1.ImagePolicy{}
		if err := c.Get(context.Background(), client.ObjectKey{Namespace: "payments", Name: "api"}, policy); err != nil {
			t.Fatal(err)
		}
		return policy.GetAnnotations()[RequestedAtAnnotation]
	}

	if code := post(r, "/hooks/imagepolicies/payments/api", `{}`, map[string]string{"Authorization": "Bearer wrong"}); code != http.StatusUnauthorized || requested() != "" {
		t.Fatalf("wrong token: code = %d", code)
	}
	if code := post(r, "/hooks/imagepolicies/payments/no-hook", `{}`, map[string]string{"Authorization": "Bearer reg-token"}); code != http.StatusNotFound {
		t.Fatalf("policy without webhook: code = %d", code)
	}
	if code := post(r, "/hooks/imagepolicies/payments/api", `{"action":"published"}`, map[string]string{"Authorization": "Bearer reg-token"}); code != http.StatusAccepted || requested() == "" {
		t.Fatalf("valid token: code = %d, annotation = %q", code, requested())
	}
}
