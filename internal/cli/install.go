package cli

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	repoURL  = "https://github.com/azrtydxb/kuvryn-sync"
	imageRef = "ghcr.io/azrtydxb/kuvryn-sync"
)

// releaseTag matches a version a release was built from: a tag such as
// v0.6.2 or v0.7.0-rc.1. "dev", "sha-<commit>" and a "-dirty" build have no
// release to install from.
var releaseTag = regexp.MustCompile(`^v\d+\.\d+\.\d+(-[0-9A-Za-z.]+)?$`)

// renderInstall prints the commands that install the release the binary was
// built from, which work without a checkout of the repository.
func renderInstall(version string) string {
	var b strings.Builder
	if !releaseTag.MatchString(version) || strings.HasSuffix(version, "-dirty") {
		fmt.Fprintf(&b, "This ksync build (%s) is unreleased, so it has no install bundle.\n", orUnknown(version))
		fmt.Fprintf(&b, "Install a release, or build one from source, as %s/blob/main/docs/install.md describes.\n", repoURL)
		return b.String()
	}
	fmt.Fprintf(&b, `Install Kuvryn Sync %[1]s. cert-manager must be running first.

With the release manifests:

  kubectl apply -f %[2]s/releases/download/%[1]s/install.yaml

Or with the Helm chart, from a checkout of the release tag:

  git clone --depth 1 --branch %[1]s %[2]s.git
  cd kuvryn-sync
  kubectl apply -f config/crd/bases
  helm upgrade --install kuvryn-sync charts/kuvryn-sync \
    --namespace kuvryn-sync-system --create-namespace

The image %[3]s:%[1]s may be private: create a pull Secret and set
image.pullSecrets with Helm, or patch the Deployment with the raw manifests.
See %[2]s/blob/%[1]s/docs/install.md
`, version, repoURL, imageRef)
	return b.String()
}
