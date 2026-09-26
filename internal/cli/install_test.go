package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/azrtydxb/kuvryn-sync/internal/version"
)

// Catches install instructions that name repository-relative paths, such as
// dist/install.yaml, which a user who only has the binary does not have.
func TestInstallPrintsTheReleaseCommands(t *testing.T) {
	previous := version.Version
	version.Version = "v0.6.2"
	t.Cleanup(func() { version.Version = previous })

	var stdout, stderr bytes.Buffer
	handled, code := Run(context.Background(), []string{"install"}, &stdout, &stderr)
	if !handled || code != 0 {
		t.Fatalf("Run(install) = handled %v, code %d, stderr %q", handled, code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{
		"kubectl apply -f https://github.com/azrtydxb/kuvryn-sync/releases/download/v0.6.2/install.yaml",
		"git clone --depth 1 --branch v0.6.2 https://github.com/azrtydxb/kuvryn-sync.git",
		"kubectl apply -f config/crd/bases",
		"helm upgrade --install kuvryn-sync charts/kuvryn-sync",
		"cert-manager",
		"ghcr.io/azrtydxb/kuvryn-sync:v0.6.2",
		"image.pullSecrets",
		"https://github.com/azrtydxb/kuvryn-sync/blob/v0.6.2/docs/install.md",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("install output is missing %q:\n%s", want, out)
		}
	}
	for _, repoPath := range []string{"-f dist/install.yaml", "-k config/default"} {
		if strings.Contains(out, repoPath) {
			t.Errorf("install output names the checkout path %q:\n%s", repoPath, out)
		}
	}
}

// Catches a development build pointing at a release download that does not
// exist for its version.
func TestInstallForAnUnreleasedBuild(t *testing.T) {
	for _, v := range []string{"dev", "sha-37327fc", "sha-37327fc-dirty", "v0.6.2-dirty", ""} {
		out := renderInstall(v)
		if strings.Contains(out, "releases/download") || strings.Contains(out, "kubectl apply -f https://") {
			t.Errorf("renderInstall(%q) points at a release download:\n%s", v, out)
		}
		for _, want := range []string{"unreleased", "https://github.com/azrtydxb/kuvryn-sync/blob/main/docs/install.md"} {
			if !strings.Contains(out, want) {
				t.Errorf("renderInstall(%q) is missing %q:\n%s", v, want, out)
			}
		}
	}
	if out := renderInstall("v0.7.0-rc.1"); !strings.Contains(out, "/releases/download/v0.7.0-rc.1/install.yaml") {
		t.Errorf("a pre-release tag is a release:\n%s", out)
	}
}
