package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunPlanFromRevisionFile(t *testing.T) {
	revision := `apiVersion: sync.kuvryn.io/v1alpha1
kind: Revision
metadata:
  name: payments-abc
spec:
  applicationRef:
    name: payments
  source:
    repositoryRef:
      name: platform
    revision: abc123
    render:
      type: yaml
status:
  plan:
    summary:
      create: 1
      update: 1
      delete: 1
      unchanged: 3
    resources:
      - resource:
          apiVersion: v1
          kind: Secret
          namespace: payments
          name: db
        action: Update
        changes:
          - path: data.password
            before: old-secret
            after: new-secret
      - resource:
          apiVersion: v1
          kind: ConfigMap
          namespace: payments
          name: legacy
        action: Delete
        destructive: true
        warnings:
          - delete action is destructive
`
	path := filepath.Join(t.TempDir(), "revision.yaml")
	if err := os.WriteFile(path, []byte(revision), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	handled, code := Run(context.Background(), []string{"plan", "payments", "-f", path}, &stdout, &stderr)
	if !handled || code != 0 {
		t.Fatalf("handled=%v code=%d stderr=%s", handled, code, stderr.String())
	}
	text := stdout.String()
	for _, forbidden := range []string{"old-secret", "new-secret"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("plan leaked secret %q: %s", forbidden, text)
		}
	}
	for _, required := range []string{"Application: payments", "Revision:    abc123", "! DELETE ConfigMap/payments/legacy", "1 deleted", "REDACTED"} {
		if !strings.Contains(text, required) {
			t.Fatalf("plan output missing %q: %s", required, text)
		}
	}
}

func TestRunPlanJSONFromRevisionFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "revision.yaml")
	if err := os.WriteFile(path, []byte(`apiVersion: sync.kuvryn.io/v1alpha1
kind: Revision
spec:
  applicationRef:
    name: payments
  source:
    repositoryRef:
      name: platform
    revision: abc123
    render:
      type: yaml
status:
  plan:
    summary:
      unchanged: 1
`), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	handled, code := Run(context.Background(), []string{"plan", "payments", "-f", path, "-o", "json"}, &stdout, &stderr)
	if !handled || code != 0 {
		t.Fatalf("handled=%v code=%d stderr=%s", handled, code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"revision": "abc123"`) || !strings.Contains(stdout.String(), `"unchanged": 1`) {
		t.Fatalf("unexpected json output: %s", stdout.String())
	}
}
