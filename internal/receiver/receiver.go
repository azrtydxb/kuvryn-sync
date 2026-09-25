// Package receiver turns GitHub and GitLab push webhooks into immediate
// Repository fetches.
package receiver

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/time/rate"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	corev1alpha1 "github.com/azrtydxb/kuvryn-sync/api/v1alpha1"
)

// RequestedAtAnnotation is stamped on a Repository to request a fetch.
const RequestedAtAnnotation = "solder.io/reconcile-requested-at"

const maxBody = 1 << 20

var requests = prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: "solder_webhook_receiver_requests_total",
	Help: "Webhook receiver requests by result.",
}, []string{"result"})

func init() {
	metrics.Registry.MustRegister(requests)
}

// Receiver serves POST /hooks/{namespace}/{name}.
type Receiver struct {
	Client client.Client
	Addr   string
	// Limit and Burst bound authenticated requests per Repository or
	// ImagePolicy; zero means 1/s, burst 10.
	Limit rate.Limit
	Burst int
	// PeerLimit and PeerBurst bound requests per remote address before
	// authentication; zero means 5/s, burst 50.
	PeerLimit rate.Limit
	PeerBurst int

	mu       sync.Mutex
	limiters map[string]*rate.Limiter
	peers    map[string]*rate.Limiter
}

// maxPeers bounds the per-address limiters; the set is reset when it fills.
const maxPeers = 10000

// Start serves until ctx is done; it is a manager.Runnable.
func (r *Receiver) Start(ctx context.Context) error {
	server := &http.Server{Addr: r.Addr, Handler: r.Handler(), ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second}
	errs := make(chan error, 1)
	go func() { errs <- server.ListenAndServe() }()
	select {
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return server.Shutdown(shutdown)
	case err := <-errs:
		return err
	}
}

// NeedLeaderElection is false: every replica behind the Service must answer,
// and any of them may request a fetch.
func (r *Receiver) NeedLeaderElection() bool { return false }

// Handler returns the receiver's HTTP handler.
func (r *Receiver) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /hooks/{namespace}/{name}", r.receive)
	mux.HandleFunc("POST /hooks/imagepolicies/{namespace}/{name}", r.receiveImage)
	return mux
}

func (r *Receiver) receive(w http.ResponseWriter, req *http.Request) {
	repository := &corev1alpha1.Repository{}
	body, ok := r.admit(w, req, "repository", repository, func() string {
		if repository.Spec.Webhook == nil || repository.Spec.Git == nil {
			return ""
		}
		return repository.Spec.Webhook.SecretRef.Name
	})
	if !ok {
		return
	}
	switch event := req.Header.Get("X-GitHub-Event") + req.Header.Get("X-Gitlab-Event"); event {
	case "ping":
		reply(w, http.StatusOK, "ping")
		return
	case "push", "Push Hook", "Tag Push Hook":
	default:
		reply(w, http.StatusAccepted, "ignored")
		return
	}
	if !matches(body, repository.Spec.Git.URL) {
		reply(w, http.StatusBadRequest, "repository_mismatch")
		return
	}
	if err := requestReconcile(req.Context(), r.Client, repository); err != nil {
		reply(w, http.StatusInternalServerError, "error")
		return
	}
	reply(w, http.StatusAccepted, "accepted")
}

// receiveImage requests an immediate scan of an ImagePolicy. Any
// authenticated request counts, since registries differ in what they send.
func (r *Receiver) receiveImage(w http.ResponseWriter, req *http.Request) {
	policy := &corev1alpha1.ImagePolicy{}
	if _, ok := r.admit(w, req, "imagepolicy", policy, func() string {
		if policy.Spec.Webhook == nil {
			return ""
		}
		return policy.Spec.Webhook.SecretRef.Name
	}); !ok {
		return
	}
	if err := requestReconcile(req.Context(), r.Client, policy); err != nil {
		reply(w, http.StatusInternalServerError, "error")
		return
	}
	reply(w, http.StatusAccepted, "accepted")
}

// admit runs the checks every hook shares: the caller's rate limit, the body
// size, and authentication against the token in the Secret that secretName
// picks from obj once it is loaded. An unknown object and a bad token get the
// same answer, so callers cannot probe which objects exist. The object's own
// rate limit is charged only after authentication, so strangers cannot use up
// its budget. When admit returns false it has already replied.
func (r *Receiver) admit(w http.ResponseWriter, req *http.Request, kind string, obj client.Object, secretName func() string) ([]byte, bool) {
	if !r.allowPeer(req) {
		reply(w, http.StatusTooManyRequests, "rate_limited")
		return nil, false
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, req.Body, maxBody))
	if err != nil {
		if errors.As(err, new(*http.MaxBytesError)) {
			reply(w, http.StatusRequestEntityTooLarge, "too_large")
		} else {
			reply(w, http.StatusBadRequest, "bad_request")
		}
		return nil, false
	}
	key := client.ObjectKey{Namespace: req.PathValue("namespace"), Name: req.PathValue("name")}
	token, err := r.token(req.Context(), key, obj, secretName)
	if err != nil {
		reply(w, http.StatusInternalServerError, "error")
		return nil, false
	}
	if len(token) == 0 || !authentic(req, body, token) {
		reply(w, http.StatusUnauthorized, "unauthorized")
		return nil, false
	}
	// Limiters exist only for authenticated objects, so unknown paths cannot grow them.
	if !r.allowObject(kind + "/" + key.String()) {
		reply(w, http.StatusTooManyRequests, "rate_limited")
		return nil, false
	}
	return body, true
}

// token loads obj and the webhook token it names. A missing object, webhook,
// Secret, or token yields no token and no error.
func (r *Receiver) token(ctx context.Context, key client.ObjectKey, obj client.Object, secretName func() string) ([]byte, error) {
	if err := r.Client.Get(ctx, key, obj); err != nil {
		return nil, client.IgnoreNotFound(err)
	}
	name := secretName()
	if name == "" {
		return nil, nil
	}
	secret := &corev1.Secret{}
	if err := r.Client.Get(ctx, client.ObjectKey{Namespace: key.Namespace, Name: name}, secret); err != nil {
		return nil, client.IgnoreNotFound(err)
	}
	return secret.Data["token"], nil
}

// requestReconcile stamps RequestedAtAnnotation, which re-queues obj.
func requestReconcile(ctx context.Context, c client.Client, obj client.Object) error {
	patch := client.MergeFrom(obj.DeepCopyObject().(client.Object))
	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[RequestedAtAnnotation] = time.Now().UTC().Format(time.RFC3339Nano)
	obj.SetAnnotations(annotations)
	return c.Patch(ctx, obj, patch)
}

func (r *Receiver) allowObject(key string) bool {
	limit, burst := r.Limit, r.Burst
	if limit == 0 {
		limit, burst = rate.Every(time.Second), 10
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.limiters == nil {
		r.limiters = map[string]*rate.Limiter{}
	}
	return take(r.limiters, key, limit, burst)
}

func (r *Receiver) allowPeer(req *http.Request) bool {
	peer, _, err := net.SplitHostPort(req.RemoteAddr)
	if err != nil {
		peer = req.RemoteAddr
	}
	limit, burst := r.PeerLimit, r.PeerBurst
	if limit == 0 {
		limit, burst = 5, 50
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.peers == nil || len(r.peers) >= maxPeers {
		r.peers = map[string]*rate.Limiter{}
	}
	return take(r.peers, peer, limit, burst)
}

// take charges one request to the limiter for key, creating it on first use.
func take(limiters map[string]*rate.Limiter, key string, limit rate.Limit, burst int) bool {
	limiter, ok := limiters[key]
	if !ok {
		limiter = rate.NewLimiter(limit, burst)
		limiters[key] = limiter
	}
	return limiter.Allow()
}

// authentic verifies a GitHub HMAC signature, a GitLab token, or a Bearer
// token in constant time.
func authentic(req *http.Request, body, token []byte) bool {
	if signature := req.Header.Get("X-Hub-Signature-256"); signature != "" {
		mac := hmac.New(sha256.New, token)
		mac.Write(body)
		return hmac.Equal([]byte(signature), []byte("sha256="+hex.EncodeToString(mac.Sum(nil))))
	}
	if gitlab := req.Header.Get("X-Gitlab-Token"); gitlab != "" {
		return subtle.ConstantTimeCompare([]byte(gitlab), token) == 1
	}
	if bearer, ok := strings.CutPrefix(req.Header.Get("Authorization"), "Bearer "); ok && bearer != "" {
		return subtle.ConstantTimeCompare([]byte(bearer), token) == 1
	}
	return false
}

// matches reports whether the push payload names the Repository's URL.
func matches(body []byte, repositoryURL string) bool {
	var payload struct {
		Repository struct {
			CloneURL string `json:"clone_url"`
			SSHURL   string `json:"ssh_url"`
			HTMLURL  string `json:"html_url"`
		} `json:"repository"`
		Project struct {
			HTTPURL string `json:"git_http_url"`
			SSHURL  string `json:"git_ssh_url"`
			WebURL  string `json:"web_url"`
		} `json:"project"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return false
	}
	want := normalize(repositoryURL)
	for _, candidate := range []string{payload.Repository.CloneURL, payload.Repository.SSHURL, payload.Repository.HTMLURL, payload.Project.HTTPURL, payload.Project.SSHURL, payload.Project.WebURL} {
		if candidate != "" && normalize(candidate) == want {
			return true
		}
	}
	return false
}

// normalize reduces https, ssh, and scp-style Git URLs to host/path.
func normalize(raw string) string {
	raw = strings.TrimSpace(raw)
	if !strings.Contains(raw, "://") && strings.Contains(raw, "@") && strings.Contains(raw, ":") {
		raw = "ssh://" + strings.Replace(raw, ":", "/", 1)
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	return strings.ToLower(parsed.Hostname()) + "/" + strings.TrimSuffix(strings.Trim(parsed.Path, "/"), ".git")
}

func reply(w http.ResponseWriter, status int, result string) {
	requests.WithLabelValues(result).Inc()
	w.WriteHeader(status)
	_, _ = w.Write([]byte(result + "\n"))
}
