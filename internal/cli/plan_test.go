package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	corev1alpha1 "github.com/azrtydxb/kuvryn-sync/api/v1alpha1"
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

// Catches `ksync plan` showing the newest-created Revision rather than the
// one the Application wants: during a rollback it showed the Healthy
// Revision being rolled back from, with no approval hint, while the target
// waited for approval.
func TestPlanShowsTheRevisionTheApplicationWants(t *testing.T) {
	revision := func(name, commit string, phase corev1alpha1.RevisionPhase, minute int, held bool) corev1alpha1.Revision {
		rev := rollbackRevision(name, commit, phase, minute)
		rev.Namespace, rev.Spec.ApplicationRef.Name = "ksync-demo", "podinfo"
		if held {
			rev = heldRevision(rev)
			rev.Namespace = "ksync-demo"
		}
		return rev
	}
	app := func(desired, deployed string, rollbackTo string) *corev1alpha1.Application {
		a := &corev1alpha1.Application{ObjectMeta: metav1.ObjectMeta{Name: "podinfo", Namespace: "ksync-demo"}}
		a.Status.DesiredRevision, a.Status.DeployedRevision = desired, deployed
		if rollbackTo != "" {
			a.Annotations = map[string]string{corev1alpha1.RollbackRevisionAnnotation: rollbackTo, corev1alpha1.RollbackFromAnnotation: desired}
		}
		return a
	}
	target := revision("podinfo-b939e830aae1", "a30f", corev1alpha1.RevisionPhaseAwaitingApproval, 0, false)
	current := revision("podinfo-1a77a0f91d12", "dd50", corev1alpha1.RevisionPhaseHealthy, 1, false)
	cases := []struct {
		name      string
		app       *corev1alpha1.Application
		revisions []corev1alpha1.Revision
		want      string
	}{
		{"a rollback requested and not yet reconciled", app("dd50", "dd50", "a30f"), []corev1alpha1.Revision{target, current}, target.Name},
		{"a rollback target awaiting approval", app("a30f", "dd50", "a30f"), []corev1alpha1.Revision{target, current}, target.Name},
		{"a held commit keeps the deployed Revision", app("dd50", "a30f", ""), []corev1alpha1.Revision{
			revision("podinfo-b939e830aae1", "a30f", corev1alpha1.RevisionPhaseRolledBack, 0, false),
			revision("podinfo-1a77a0f91d12", "dd50", corev1alpha1.RevisionPhaseFailed, 1, true),
		}, target.Name},
		{"a new commit awaiting approval", app("ee01", "dd50", ""), []corev1alpha1.Revision{
			current, revision("podinfo-0c0c0c0c0c0c", "ee01", corev1alpha1.RevisionPhaseAwaitingApproval, 2, false),
		}, "podinfo-0c0c0c0c0c0c"},
		{"the newest Revision of the desired commit", app("dd50", "dd50", ""), []corev1alpha1.Revision{
			target, current, revision("podinfo-ffffffffffff", "dd50", corev1alpha1.RevisionPhaseAwaitingApproval, 3, false),
		}, "podinfo-ffffffffffff"},
		{"nothing resolved yet falls back to the newest", app("", "", ""), []corev1alpha1.Revision{target, current}, current.Name},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := rollbackClient(t, tc.app, tc.revisions...)
			got, err := wantedRevision(context.Background(), c, "podinfo", "ksync-demo")
			if err != nil || got.Name != tc.want {
				name := ""
				if got != nil {
					name = got.Name
				}
				t.Fatalf("revision = %q, %v; want %q", name, err, tc.want)
			}
		})
	}

	// Without the Application, the newest Revision is all there is to show.
	c := rollbackClient(t, &corev1alpha1.Application{ObjectMeta: metav1.ObjectMeta{Name: "other", Namespace: "ksync-demo"}}, target, current)
	if got, err := wantedRevision(context.Background(), c, "podinfo", "ksync-demo"); err != nil || got.Name != current.Name {
		t.Fatalf("without the Application: %v, %v", got, err)
	}
}
