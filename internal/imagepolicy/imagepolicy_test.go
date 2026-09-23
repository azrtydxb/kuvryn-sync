package imagepolicy

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"golang.org/x/time/rate"
	"oras.land/oras-go/v2/registry/remote/auth"

	corev1alpha1 "github.com/azrtydxb/solder/api/v1alpha1"
	"github.com/azrtydxb/solder/internal/imagepolicy/registrytest"
)

const digestA = "sha256:1111111111111111111111111111111111111111111111111111111111111111"

func TestSelect(t *testing.T) {
	tags := []string{"1.2.0", "v1.10.1", "1.9.9", "2.0.0", "main-12", "main-9", "latest", "1.10.2-rc.1"}
	cases := map[string]struct {
		policy corev1alpha1.ImageSelectionPolicy
		want   string
	}{
		"semver highest in range":  {corev1alpha1.ImageSelectionPolicy{Semver: &corev1alpha1.SemverPolicy{Range: ">=1.0.0 <2.0.0"}}, "v1.10.1"},
		"numerical capture group":  {corev1alpha1.ImageSelectionPolicy{TagPattern: &corev1alpha1.TagPatternPolicy{Regex: `^main-(\d+)$`, Order: "numerical"}}, "main-12"},
		"alphabetical tag pattern": {corev1alpha1.ImageSelectionPolicy{TagPattern: &corev1alpha1.TagPatternPolicy{Regex: `^main-`, Order: "alphabetical"}}, "main-9"},
		"digest follows fixed tag": {corev1alpha1.ImageSelectionPolicy{Digest: &corev1alpha1.DigestPolicy{Tag: "latest"}}, "latest"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := Select(tc.policy, tags)
			if err != nil || got != tc.want {
				t.Fatalf("Select = %q, %v; want %q", got, err, tc.want)
			}
		})
	}
	if _, err := Select(corev1alpha1.ImageSelectionPolicy{Semver: &corev1alpha1.SemverPolicy{Range: ">=3.0.0"}}, tags); err == nil {
		t.Fatal("selected a tag outside the range")
	}
}

func TestRegistryListsTagsAndResolvesDigestsWithCredentials(t *testing.T) {
	registry := registrytest.New("robot", "pw")
	defer registry.Close()
	registry.Push("acme/api", "1.0.0", digestA)
	image := registry.Host() + "/acme/api"
	config := `{"auths":{"` + registry.Host() + `":{"auth":"` + base64.StdEncoding.EncodeToString([]byte("robot:pw")) + `"}}}`
	cred, err := Credentials([]byte(config), image)
	if err != nil {
		t.Fatal(err)
	}
	r := &Registry{PlainHTTP: true}

	tags, err := r.Tags(context.Background(), image, cred)
	if err != nil || len(tags) != 1 || tags[0] != "1.0.0" {
		t.Fatalf("tags = %v, %v", tags, err)
	}
	digest, err := r.Digest(context.Background(), image, "1.0.0", cred)
	if err != nil || digest != digestA {
		t.Fatalf("digest = %q, %v", digest, err)
	}
	if _, err := r.Tags(context.Background(), image, auth.Credential{Username: "robot", Password: "wrong"}); err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("wrong credentials: %v", err)
	}
}

func TestRegistryRateLimitsPerHost(t *testing.T) {
	registry := registrytest.New("robot", "pw")
	defer registry.Close()
	registry.Push("acme/api", "1.0.0", digestA)
	registry.Push("acme/web", "1.0.0", digestA)
	r := &Registry{PlainHTTP: true, Limit: rate.Every(time.Hour), Burst: 1}
	cred := auth.Credential{Username: "robot", Password: "pw"}
	if _, err := r.Tags(context.Background(), registry.Host()+"/acme/api", cred); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := r.Tags(ctx, registry.Host()+"/acme/web", cred); err == nil {
		t.Fatal("a second repository on the same registry bypassed the host's rate limit")
	}
}

func TestCredentialsMatchDockerConfigKeys(t *testing.T) {
	entry := `{"username":"robot","password":"pw"}`
	cases := map[string]struct {
		key, image string
		match      bool
	}{
		"trailing slash":        {"ghcr.io/", "ghcr.io/acme/api", true},
		"scheme and path":       {"https://ghcr.io/v2/", "ghcr.io/acme/api", true},
		"legacy docker hub key": {"https://index.docker.io/v1/", "docker.io/library/nginx", true},
		"docker hub by index":   {"docker.io", "index.docker.io/library/nginx", true},
		"port":                  {"localhost:5000", "localhost:5000/acme/api", true},
		"other port":            {"localhost:5001", "localhost:5000/acme/api", false},
		"suffix host":           {"ghcr.io.evil.example", "ghcr.io/acme/api", false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cred, err := Credentials([]byte(`{"auths":{"`+tc.key+`":`+entry+`}}`), tc.image)
			if tc.match && (err != nil || cred.Username != "robot") {
				t.Fatalf("key %q did not match %s: %v", tc.key, tc.image, err)
			}
			if !tc.match && err == nil {
				t.Fatalf("key %q matched %s", tc.key, tc.image)
			}
		})
	}
}
