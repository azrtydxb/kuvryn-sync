package cli

import (
	"bytes"
	"context"
	"encoding/json"
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
	for _, required := range []string{"Application: payments", "Revision:    payments-abc", "Commit:      abc123", "! DELETE ConfigMap/payments/legacy", "1 deleted", "REDACTED"} {
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

const awaitingRevision = `apiVersion: sync.kuvryn.io/v1alpha1
kind: Revision
metadata:
  name: podinfo-b939e830aae1
  namespace: ksync-demo
spec:
  applicationRef:
    name: podinfo
  source:
    repositoryRef:
      name: podinfo
    revision: b939e830aae1c0ffee
    render:
      type: yaml
status:
  phase: AwaitingApproval
  plan:
    digest: sha256:1234
    summary:
      create: 2
`

func writeRevision(t *testing.T, doc string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "revision.yaml")
	if err := os.WriteFile(path, []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// Catches a plan that names only the commit: ksync sync needs the Revision
// object's name, so an approver could not act on the plan they just read.
func TestRunPlanShowsTheRevisionToApprove(t *testing.T) {
	path := writeRevision(t, awaitingRevision)
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"plan", "podinfo", "-f", path}, "Approve with: ksync sync podinfo -n ksync-demo --revision podinfo-b939e830aae1\n"},
		{[]string{"plan", "podinfo", "-n", "ksync-demo", "-f", path}, "Approve with: ksync sync podinfo -n ksync-demo --revision podinfo-b939e830aae1\n"},
		{[]string{"plan", "podinfo", "-n", "default", "-f", path}, "Approve with: ksync sync podinfo -n default --revision podinfo-b939e830aae1\n"},
	} {
		var stdout, stderr bytes.Buffer
		handled, code := Run(context.Background(), tc.args, &stdout, &stderr)
		if !handled || code != 0 {
			t.Fatalf("%v: handled=%v code=%d stderr=%s", tc.args, handled, code, stderr.String())
		}
		text := stdout.String()
		for _, required := range []string{
			"Application: podinfo\n",
			"Revision:    podinfo-b939e830aae1\n",
			"Commit:      b939e830aae1c0ffee\n",
			"Phase:       AwaitingApproval\n",
		} {
			if !strings.Contains(text, required) {
				t.Fatalf("%v: plan output missing %q:\n%s", tc.args, required, text)
			}
		}
		if !strings.HasSuffix(text, tc.want) {
			t.Fatalf("%v: plan output does not end with %q:\n%s", tc.args, tc.want, text)
		}
	}
}

// Catches an approval hint on a Revision there is nothing to approve on, and
// a -n flag the default namespace does not need.
func TestRunPlanApproveHint(t *testing.T) {
	healthy := strings.Replace(awaitingRevision, "phase: AwaitingApproval", "phase: Healthy", 1)
	var stdout, stderr bytes.Buffer
	if _, code := Run(context.Background(), []string{"plan", "podinfo", "-f", writeRevision(t, healthy)}, &stdout, &stderr); code != 0 {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
	if strings.Contains(stdout.String(), "Approve with") || !strings.Contains(stdout.String(), "Phase:       Healthy") {
		t.Fatalf("a Healthy Revision's plan:\n%s", stdout.String())
	}

	inDefault := strings.Replace(awaitingRevision, "  namespace: ksync-demo\n", "", 1)
	stdout.Reset()
	if _, code := Run(context.Background(), []string{"plan", "podinfo", "-f", writeRevision(t, inDefault)}, &stdout, &stderr); code != 0 {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
	if !strings.HasSuffix(stdout.String(), "Approve with: ksync sync podinfo --revision podinfo-b939e830aae1\n") {
		t.Fatalf("a plan in the default namespace:\n%s", stdout.String())
	}
}

// Catches a machine-readable plan that drops or renames what scripts read.
func TestRunPlanJSONNamesTheRevision(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if _, code := Run(context.Background(), []string{"plan", "podinfo", "-f", writeRevision(t, awaitingRevision), "-o", "json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
	var doc map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &doc); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, stdout.String())
	}
	for key, want := range map[string]string{
		"application":  "podinfo",
		"revision":     "b939e830aae1c0ffee",
		"revisionName": "podinfo-b939e830aae1",
		"phase":        "AwaitingApproval",
	} {
		if doc[key] != want {
			t.Errorf("%s = %v, want %q", key, doc[key], want)
		}
	}
	if strings.Contains(stdout.String(), "Approve with") {
		t.Errorf("JSON output carries the text hint:\n%s", stdout.String())
	}
}
