//go:build release

// Package release holds checks that run against a published release, not the
// source tree. Run them with RELEASE_VERSION=vX.Y.Z go test -tags release ./test/release/.
package release

import (
	"encoding/json"
	"net/http"
	"os"
	"testing"
)

// TestReleaseImageIsPublic pulls the release manifest anonymously. GHCR
// packages start private, so this fails until the package is made public.
func TestReleaseImageIsPublic(t *testing.T) {
	version := os.Getenv("RELEASE_VERSION")
	if version == "" {
		t.Fatal("set RELEASE_VERSION, for example v0.4.0")
	}
	resp, err := http.Get("https://ghcr.io/token?scope=repository:azrtydxb/kuvryn-sync:pull")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var tok struct{ Token string }
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil {
		t.Fatalf("decode the GHCR token response: %v", err)
	}
	req, err := http.NewRequest(http.MethodGet, "https://ghcr.io/v2/azrtydxb/kuvryn-sync/manifests/"+version, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+tok.Token)
	req.Header.Set("Accept", "application/vnd.oci.image.index.v1+json, application/vnd.oci.image.manifest.v1+json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("anonymous pull of %s returned %d; make the package public", version, res.StatusCode)
	}
}
