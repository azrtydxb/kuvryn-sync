// Package imagepolicy reads image tags from OCI registries and selects the
// image an ImagePolicy should run.
package imagepolicy

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/Masterminds/semver/v3"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/time/rate"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/retry"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	corev1alpha1 "github.com/azrtydxb/kuvryn-sync/api/v1alpha1"
)

// Registry reads tags and digests from an image repository. Requests to each
// registry host share one rate limit, however many ImagePolicies point at it.
type Registry struct {
	// PlainHTTP talks to the registry over HTTP; only tests use it.
	PlainHTTP bool
	// Limit and Burst bound requests per registry host; zero means 5/s, burst 10.
	Limit rate.Limit
	Burst int

	mu       sync.Mutex
	limiters map[string]*rate.Limiter
}

var scans = prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: "kuvryn_sync_image_scans_total",
	Help: "Registry requests made by image scans, by result.",
}, []string{"result"})

func init() {
	metrics.Registry.MustRegister(scans)
}

func (r *Registry) wait(ctx context.Context, host string) error {
	r.mu.Lock()
	if r.limiters == nil {
		r.limiters = map[string]*rate.Limiter{}
	}
	limiter, ok := r.limiters[host]
	if !ok {
		limit, burst := r.Limit, r.Burst
		if limit == 0 {
			limit, burst = 5, 10
		}
		limiter = rate.NewLimiter(limit, burst)
		r.limiters[host] = limiter
	}
	r.mu.Unlock()
	return limiter.Wait(ctx)
}

func observe(err error) error {
	result := "success"
	if err != nil {
		result = "error"
	}
	scans.WithLabelValues(result).Inc()
	return err
}

// Credentials returns the registry credential for image from a
// kubernetes.io/dockerconfigjson payload.
func Credentials(dockerConfigJSON []byte, image string) (auth.Credential, error) {
	var config struct {
		Auths map[string]struct {
			Username string `json:"username"`
			Password string `json:"password"`
			Auth     string `json:"auth"`
		} `json:"auths"`
	}
	if err := json.Unmarshal(dockerConfigJSON, &config); err != nil {
		return auth.EmptyCredential, fmt.Errorf("parse registry credentials: %w", err)
	}
	host := strings.SplitN(image, "/", 2)[0]
	want := registryHost(host)
	for registry, entry := range config.Auths {
		if registryHost(registry) != want {
			continue
		}
		if entry.Auth != "" {
			decoded, err := base64.StdEncoding.DecodeString(entry.Auth)
			if err != nil {
				return auth.EmptyCredential, fmt.Errorf("parse registry credentials: %w", err)
			}
			user, pass, _ := strings.Cut(string(decoded), ":")
			return auth.Credential{Username: user, Password: pass}, nil
		}
		return auth.Credential{Username: entry.Username, Password: entry.Password}, nil
	}
	return auth.EmptyCredential, fmt.Errorf("registry credentials have no entry for %s", host)
}

// registryHost reduces a Docker config key or image host, such as ghcr.io/,
// https://index.docker.io/v1/, or localhost:5000, to a comparable host.
func registryHost(key string) string {
	if !strings.Contains(key, "://") {
		key = "https://" + key
	}
	parsed, err := url.Parse(key)
	if err != nil {
		return ""
	}
	host := strings.ToLower(parsed.Host)
	switch host {
	case "index.docker.io", "registry-1.docker.io":
		return "docker.io"
	}
	return host
}

func (r *Registry) repository(ctx context.Context, image string, cred auth.Credential) (*remote.Repository, error) {
	repo, err := remote.NewRepository(image)
	if err != nil {
		return nil, fmt.Errorf("image %s: %w", image, err)
	}
	repo.PlainHTTP = r.PlainHTTP
	repo.Client = &auth.Client{Client: retry.DefaultClient, Cache: auth.NewCache(), Credential: auth.StaticCredential(repo.Reference.Registry, cred)}
	if err := r.wait(ctx, repo.Reference.Registry); err != nil {
		return nil, err
	}
	return repo, nil
}

// Tags lists every tag of image.
func (r *Registry) Tags(ctx context.Context, image string, cred auth.Credential) ([]string, error) {
	repo, err := r.repository(ctx, image, cred)
	if err != nil {
		return nil, err
	}
	tags := []string{}
	if err := observe(repo.Tags(ctx, "", func(page []string) error {
		tags = append(tags, page...)
		return nil
	})); err != nil {
		return nil, fmt.Errorf("list tags of %s: %w", image, err)
	}
	return tags, nil
}

// Digest resolves tag to its manifest digest.
func (r *Registry) Digest(ctx context.Context, image, tag string, cred auth.Credential) (string, error) {
	repo, err := r.repository(ctx, image, cred)
	if err != nil {
		return "", err
	}
	desc, err := repo.Resolve(ctx, tag)
	if err := observe(err); err != nil {
		return "", fmt.Errorf("resolve %s:%s: %w", image, tag, err)
	}
	return desc.Digest.String(), nil
}

// Select returns the tag a policy chooses from tags.
func Select(policy corev1alpha1.ImageSelectionPolicy, tags []string) (string, error) {
	switch {
	case policy.Digest != nil:
		return policy.Digest.Tag, nil
	case policy.Semver != nil:
		constraint, err := semver.NewConstraint(policy.Semver.Range)
		if err != nil {
			return "", fmt.Errorf("semver range %q: %w", policy.Semver.Range, err)
		}
		var best *semver.Version
		var bestTag string
		for _, tag := range tags {
			version, err := semver.NewVersion(tag)
			if err != nil || !constraint.Check(version) {
				continue
			}
			if best == nil || version.GreaterThan(best) {
				best, bestTag = version, tag
			}
		}
		if best == nil {
			return "", fmt.Errorf("no tag satisfies %q", policy.Semver.Range)
		}
		return bestTag, nil
	case policy.TagPattern != nil:
		pattern, err := regexp.Compile(policy.TagPattern.Regex)
		if err != nil {
			return "", fmt.Errorf("tag pattern %q: %w", policy.TagPattern.Regex, err)
		}
		type candidate struct {
			tag string
			key float64
		}
		matches := []candidate{}
		for _, tag := range tags {
			groups := pattern.FindStringSubmatch(tag)
			if groups == nil {
				continue
			}
			c := candidate{tag: tag}
			if policy.TagPattern.Order == "numerical" {
				value := groups[0]
				if len(groups) > 1 {
					value = groups[1]
				}
				if c.key, err = strconv.ParseFloat(value, 64); err != nil {
					continue
				}
			}
			matches = append(matches, c)
		}
		if len(matches) == 0 {
			return "", fmt.Errorf("no tag matches %q", policy.TagPattern.Regex)
		}
		sort.SliceStable(matches, func(i, j int) bool {
			if policy.TagPattern.Order == "numerical" {
				return matches[i].key < matches[j].key
			}
			return matches[i].tag < matches[j].tag
		})
		return matches[len(matches)-1].tag, nil
	default:
		return "", fmt.Errorf("the policy selects nothing")
	}
}
