package console

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// markdownSection returns the text under the first heading line equal to
// heading, up to the next heading of the same or a higher level, and the
// heading's position in the file; -1 when there is no such heading.
func markdownSection(doc, heading string) (string, int) {
	level := strings.Index(heading, " ")
	lines := strings.Split(doc, "\n")
	fenced := false
	for i, line := range lines {
		if strings.HasPrefix(line, "```") {
			fenced = !fenced
		}
		if fenced || strings.TrimSpace(line) != heading {
			continue
		}
		var body []string
		inner := false
		for _, next := range lines[i+1:] {
			if strings.HasPrefix(next, "```") {
				inner = !inner
			}
			if !inner && strings.HasPrefix(next, "#") {
				if l := strings.Index(next, " "); l > 0 && l <= level && strings.Trim(next[:l], "#") == "" {
					break
				}
			}
			body = append(body, next)
		}
		return strings.Join(body, "\n"), i
	}
	return "", -1
}

// TestConsoleDocsCoverTokenSignIn fails if docs/console.md lacks the token
// sign-in, viewer-token or raw-manifest sections, or leads with OIDC, or if
// docs/security.md does not say what a token session can do.
func TestConsoleDocsCoverTokenSignIn(t *testing.T) {
	read := func(name string) string {
		t.Helper()
		b, err := os.ReadFile(filepath.Join("..", "..", "docs", name))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	doc := read("console.md")
	token, tokenAt := markdownSection(doc, "## Sign in with a Kubernetes token")
	viewer, _ := markdownSection(doc, "## Create a viewer token")
	raw, _ := markdownSection(doc, "## Add the console to a raw-manifest install")
	_, dexAt := markdownSection(doc, "## Set up OIDC sign-in with Dex")
	for name, tc := range map[string]struct {
		body string
		want []string
	}{
		"## Sign in with a Kubernetes token":           {token, []string{"SelfSubjectReview", "8 hours", "POST /auth/token", "1.28"}},
		"## Create a viewer token":                     {viewer, []string{"kind: ServiceAccount", "kind: RoleBinding", "kubectl create token", "sync.kuvryn.io", "--duration"}},
		"## Add the console to a raw-manifest install": {raw, []string{"dist/install.yaml", "helm template kuvryn-sync charts/kuvryn-sync", "--set console.enabled=true", "--show-only templates/console.yaml", "kubectl apply -n kuvryn-sync-system -f -"}},
	} {
		if tc.body == "" {
			t.Errorf("docs/console.md has no %q section", name)
			continue
		}
		for _, want := range tc.want {
			if !strings.Contains(tc.body, want) {
				t.Errorf("docs/console.md %q does not mention %q", name, want)
			}
		}
	}
	if tokenAt < 0 || dexAt < 0 || tokenAt > dexAt {
		t.Errorf("docs/console.md must lead with token sign-in and keep OIDC under \"## Set up OIDC sign-in with Dex\" after it (token at line %d, Dex at %d)", tokenAt, dexAt)
	}
	security, _ := markdownSection(read("security.md"), "## Web console")
	for _, want := range []string{"token session", "bearer token", "impersonat", "8 hours"} {
		if !strings.Contains(security, want) {
			t.Errorf("docs/security.md \"## Web console\" does not mention %q", want)
		}
	}
}
