package brand

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestNoSolderNameRemains fails while an old product name is left in the
// listed scopes. Later tasks widen scopes until it covers the repository.
func TestNoSolderNameRemains(t *testing.T) {
	root := "../.."
	out, err := exec.Command("git", "-C", root, "grep", "-n", "github.com/azrtydxb/solder", "--", "*.go", "go.mod", "PROJECT", ":!internal/brand/names_test.go").CombinedOutput()
	if err == nil {
		t.Fatalf("old module path remains:\n%s", out)
	}
	if len(strings.TrimSpace(string(out))) != 0 && !strings.Contains(string(out), "exit status 1") {
		t.Fatalf("git grep failed: %s", out)
	}
	_ = os.Getenv

	out, err = exec.Command("git", "-C", root, "grep", "-n", `solder\.io/`, "--", "*.go", "config", "charts", ":!config/crd/bases", ":!internal/brand/names_test.go").CombinedOutput()
	if err == nil {
		t.Fatalf("old key prefix remains:\n%s", out)
	}
	if len(strings.TrimSpace(string(out))) != 0 {
		t.Fatalf("git grep failed: %s", out)
	}
}
