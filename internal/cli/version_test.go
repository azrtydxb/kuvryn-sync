package cli

import (
	"bytes"
	"context"
	"testing"

	"github.com/azrtydxb/solder/internal/version"
)

// Catches a version command that prints a fixed string instead of the version
// embedded at build time.
func TestRunVersionPrintsBuildVersion(t *testing.T) {
	previous := version.Version
	version.Version = "v9.8.7-test"
	t.Cleanup(func() { version.Version = previous })

	var stdout, stderr bytes.Buffer
	handled, code := Run(context.Background(), []string{"version"}, &stdout, &stderr)
	if !handled || code != 0 {
		t.Fatalf("Run(version) = handled %v, code %d, stderr %q", handled, code, stderr.String())
	}
	if got, want := stdout.String(), "solder v9.8.7-test\n"; got != want {
		t.Fatalf("version output = %q, want %q", got, want)
	}
}
