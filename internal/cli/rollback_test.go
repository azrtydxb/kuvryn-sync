package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	corev1alpha1 "github.com/azrtydxb/kuvryn-sync/api/v1alpha1"
	"github.com/azrtydxb/kuvryn-sync/internal/applier"
	"github.com/azrtydxb/kuvryn-sync/internal/revisionid"
)

var rollbackEpoch = time.Unix(1000, 0)

func rollbackRevision(name, source string, phase corev1alpha1.RevisionPhase, minute int) corev1alpha1.Revision {
	return corev1alpha1.Revision{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default", CreationTimestamp: metav1.NewTime(rollbackEpoch.Add(time.Duration(minute) * time.Minute))},
		Spec:       corev1alpha1.RevisionSpec{ApplicationRef: corev1alpha1.LocalObjectReference{Name: "payments"}, Source: corev1alpha1.RevisionSource{Revision: source}},
		Status:     corev1alpha1.RevisionStatus{Phase: phase},
	}
}

func heldRevision(rev corev1alpha1.Revision) corev1alpha1.Revision {
	rev.Status.Phase = corev1alpha1.RevisionPhaseFailed
	rev.Status.Conditions = []metav1.Condition{{Type: corev1alpha1.RolledBackCondition, Status: metav1.ConditionTrue, Reason: "ManualRollback"}}
	return rev
}

func rollbackApp(desired, deployed string) *corev1alpha1.Application {
	app := &corev1alpha1.Application{ObjectMeta: metav1.ObjectMeta{Name: "payments", Namespace: "default"}}
	app.Status.DesiredRevision, app.Status.DeployedRevision = desired, deployed
	return app
}

// Catches a default target that is the deployed or desired revision, which
// made `ksync rollback` without --revision do nothing, and one that ignores
// earlier rollback targets.
func TestDefaultRollbackTarget(t *testing.T) {
	other := rollbackRevision("search-z", "z-sha", corev1alpha1.RevisionPhaseHealthy, 9)
	other.Spec.ApplicationRef.Name = "search"
	cases := []struct {
		name      string
		app       *corev1alpha1.Application
		revisions []corev1alpha1.Revision
		want      string
	}{
		{
			name: "skips the deployed and desired revisions",
			app:  rollbackApp("c-sha", "c-sha"),
			revisions: []corev1alpha1.Revision{
				rollbackRevision("payments-a", "a-sha", corev1alpha1.RevisionPhaseHealthy, 0),
				rollbackRevision("payments-b", "b-sha", corev1alpha1.RevisionPhaseHealthy, 1),
				rollbackRevision("payments-c", "c-sha", corev1alpha1.RevisionPhaseHealthy, 2),
				other,
			},
			want: "payments-b",
		},
		{
			name: "a failed render leaves the desired revision undeployed",
			app:  rollbackApp("c-sha", "b-sha"),
			revisions: []corev1alpha1.Revision{
				rollbackRevision("payments-a", "a-sha", corev1alpha1.RevisionPhaseHealthy, 0),
				rollbackRevision("payments-b", "b-sha", corev1alpha1.RevisionPhaseHealthy, 1),
				rollbackRevision("payments-c", "c-sha", corev1alpha1.RevisionPhaseFailed, 2),
			},
			want: "payments-a",
		},
		{
			name: "an earlier rollback target is known good",
			app:  rollbackApp("c-sha", "c-sha"),
			revisions: []corev1alpha1.Revision{
				rollbackRevision("payments-a", "a-sha", corev1alpha1.RevisionPhaseHealthy, 0),
				rollbackRevision("payments-b", "b-sha", corev1alpha1.RevisionPhaseRolledBack, 1),
				heldRevision(rollbackRevision("payments-x", "x-sha", corev1alpha1.RevisionPhaseFailed, 3)),
				rollbackRevision("payments-c", "c-sha", corev1alpha1.RevisionPhaseHealthy, 2),
			},
			want: "payments-b",
		},
		{
			name: "ties on creation time break by name",
			app:  rollbackApp("c-sha", "c-sha"),
			revisions: []corev1alpha1.Revision{
				rollbackRevision("payments-b", "b-sha", corev1alpha1.RevisionPhaseHealthy, 1),
				rollbackRevision("payments-a", "a-sha", corev1alpha1.RevisionPhaseHealthy, 1),
			},
			want: "payments-b",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := defaultRollbackTarget(tc.app, tc.revisions)
			if err != nil || got != tc.want {
				t.Fatalf("target = %q, %v; want %q", got, err, tc.want)
			}
		})
	}

	app := rollbackApp("a-sha", "a-sha")
	if got, err := defaultRollbackTarget(app, []corev1alpha1.Revision{rollbackRevision("payments-a", "a-sha", corev1alpha1.RevisionPhaseHealthy, 0)}); err == nil {
		t.Fatalf("target = %q; want an error when only the deployed revision is Healthy", got)
	}
}

func rollbackClient(t *testing.T, app *corev1alpha1.Application, revisions ...corev1alpha1.Revision) client.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := corev1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	objects := make([]client.Object, 0, 1+len(revisions))
	objects = append(objects, app)
	for i := range revisions {
		// The controller labels every Revision with its Application, and the
		// CLI lists them by that label.
		labelled := revisions[i].DeepCopy()
		labelled.Labels = map[string]string{applier.ApplicationLabelKey: labelled.Spec.ApplicationRef.Name}
		objects = append(objects, labelled)
	}
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()
}

// Catches a rollback request that does not record what it rolls back from,
// which the controller needs to hold the replaced revision.
func TestRollbackRecordsTheDesiredRevisionItRollsBackFrom(t *testing.T) {
	c := rollbackClient(t, rollbackApp("c-sha", "b-sha"),
		rollbackRevision("payments-a", "a-sha", corev1alpha1.RevisionPhaseHealthy, 0),
		rollbackRevision("payments-b", "b-sha", corev1alpha1.RevisionPhaseHealthy, 1),
		rollbackRevision("payments-c", "c-sha", corev1alpha1.RevisionPhaseFailed, 2))
	var stdout bytes.Buffer
	if err := rollback(context.Background(), c, "default", "payments", "payments-b", &stdout); err != nil {
		t.Fatal(err)
	}
	app := &corev1alpha1.Application{}
	if err := c.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "payments"}, app); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		corev1alpha1.RollbackRevisionAnnotation: "b-sha",
		corev1alpha1.RollbackFromAnnotation:     "c-sha",
		corev1alpha1.RollbackKindAnnotation:     corev1alpha1.RollbackKindManual,
	}
	for key, value := range want {
		if app.Annotations[key] != value {
			t.Errorf("annotation %s = %q, want %q", key, app.Annotations[key], value)
		}
	}
}

// Catches a rollback to another Application's Revision, and a rollback to a
// held Revision that does not say it lifts the hold.
func TestRollbackChecksItsTarget(t *testing.T) {
	other := rollbackRevision("search-a", "a-sha", corev1alpha1.RevisionPhaseHealthy, 0)
	other.Spec.ApplicationRef.Name = "search"
	c := rollbackClient(t, rollbackApp("b-sha", "a-sha"), other,
		heldRevision(rollbackRevision("payments-b", "b-sha", corev1alpha1.RevisionPhaseFailed, 1)),
		rollbackRevision("payments-c", "c-sha", corev1alpha1.RevisionPhaseHealthy, 2))
	var stdout bytes.Buffer
	if err := rollback(context.Background(), c, "default", "payments", "search-a", &stdout); err == nil || !strings.Contains(err.Error(), `belongs to application "search"`) {
		t.Fatalf("rollback to another Application's Revision: %v", err)
	}
	if err := rollback(context.Background(), c, "default", "payments", "payments-b", &stdout); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "lifts that hold") {
		t.Fatalf("rollback to a held Revision did not explain the hold: %s", stdout.String())
	}
	// Lifting the hold on the desired revision holds nothing once it is done.
	if strings.Contains(stdout.String(), "holding") {
		t.Fatalf("lifting a hold claims to hold a revision: %s", stdout.String())
	}
}

// Catches a second rollback while one is pending recording the first one's
// target as the revision rolled back from, which held the good commit and
// left the bad one to deploy again.
func TestRollbackKeepsThePendingRequestsSource(t *testing.T) {
	app := rollbackApp("b-sha", "x-sha")
	app.Annotations = map[string]string{
		corev1alpha1.RollbackRevisionAnnotation: "b-sha",
		corev1alpha1.RollbackFromAnnotation:     "x-sha",
		corev1alpha1.RollbackKindAnnotation:     corev1alpha1.RollbackKindManual,
	}
	c := rollbackClient(t, app,
		rollbackRevision("payments-a", "a-sha", corev1alpha1.RevisionPhaseHealthy, 0),
		rollbackRevision("payments-b", "b-sha", corev1alpha1.RevisionPhaseHealthy, 1),
		rollbackRevision("payments-x", "x-sha", corev1alpha1.RevisionPhaseHealthy, 2))
	var stdout bytes.Buffer
	if err := rollback(context.Background(), c, "default", "payments", "payments-a", &stdout); err != nil {
		t.Fatal(err)
	}
	updated := &corev1alpha1.Application{}
	if err := c.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "payments"}, updated); err != nil {
		t.Fatal(err)
	}
	if got := updated.Annotations[corev1alpha1.RollbackFromAnnotation]; got != "x-sha" {
		t.Fatalf("rollback-from = %q, want the pending request's x-sha", got)
	}
	if got := updated.Annotations[corev1alpha1.RollbackRevisionAnnotation]; got != "a-sha" {
		t.Fatalf("rollback-revision = %q, want a-sha", got)
	}
}

// Catches an approval of a Revision a rollback replaced, which the controller
// would never deploy.
func TestApproveRefusesARolledBackRevision(t *testing.T) {
	held := heldRevision(rollbackRevision("payments-b", "b-sha", corev1alpha1.RevisionPhaseFailed, 1))
	held.Status.Plan.Digest = "digest"
	c := rollbackClient(t, rollbackApp("b-sha", "a-sha"), held)
	var stdout bytes.Buffer
	err := approve(context.Background(), c, "default", "payments", "payments-b", &stdout)
	if err == nil || err.Error() != "revision payments-b was replaced by a rollback; push a new commit, delete the Revision, or run ksync rollback --revision payments-b" {
		t.Fatalf("err = %v", err)
	}
}

// Catches `ksync rollback` on a manual-approval Application saying only that
// it requested a rollback, which left people waiting for a deploy that
// needed a separate `ksync sync`, and claiming to deploy the Revision it
// asked for when the controller would build another one from a changed
// spec. The request names the Revision chosen, and the output says it
// approves and deploys that Revision only when the controller will.
func TestRollbackSaysWhatItApprovesAndDeploys(t *testing.T) {
	manual := func() *corev1alpha1.Application {
		app := rollbackApp("b-sha", "b-sha")
		app.Spec.Sync.Automatic = false
		app.Spec.Source.Path = "apps/payments"
		app.Status.ServiceAccountName = "payments-deployer"
		return app
	}
	// Revisions named as the controller names them for the spec now.
	built := func(app *corev1alpha1.Application, commit string, phase corev1alpha1.RevisionPhase, minute int) corev1alpha1.Revision {
		name, _, err := revisionid.For(app, commit, app.Status.ServiceAccountName)
		if err != nil {
			t.Fatal(err)
		}
		rev := rollbackRevision(name, commit, phase, minute)
		rev.Spec.DesiredStateHash = "hash-" + commit
		rev.Status.DeployedDesiredStateHash = "hash-" + commit
		return rev
	}
	revisions := func(app *corev1alpha1.Application) []corev1alpha1.Revision {
		return []corev1alpha1.Revision{built(app, "a-sha", corev1alpha1.RevisionPhaseHealthy, 0), built(app, "b-sha", corev1alpha1.RevisionPhaseHealthy, 1)}
	}
	// The admission webhook records the requester and the chosen
	// Revision's desired state; the fake client has no webhook.
	stamping := func(c client.Client) client.Client {
		return interceptor.NewClient(c.(client.WithWatch), interceptor.Funcs{
			Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
				annotations := obj.GetAnnotations()
				annotations[corev1alpha1.RollbackRequestedByAnnotation] = "alice@example.com"
				annotations[corev1alpha1.RollbackTargetHashAnnotation] = revisionid.Binding("hash-a-sha", obj.(*corev1alpha1.Application))
				obj.SetAnnotations(annotations)
				return c.Update(ctx, obj, opts...)
			},
		})
	}
	run := func(c client.Client) string {
		t.Helper()
		var stdout bytes.Buffer
		if err := rollback(context.Background(), c, "default", "payments", "", &stdout); err != nil {
			t.Fatal(err)
		}
		return stdout.String()
	}

	app := manual()
	chosen := revisions(app)[0].Name
	c := stamping(rollbackClient(t, app, revisions(app)...))
	want := "rollback requested for payments to " + chosen + " (a-sha)\n" +
		"the request approves and deploys " + chosen + " as alice@example.com while it renders what it deployed; no ksync sync is needed\n" +
		"holding b-sha once the rollback completes\n"
	if got := run(c); got != want {
		t.Fatalf("manual sync output:\n%s\nwant:\n%s", got, want)
	}
	requested := &corev1alpha1.Application{}
	if err := c.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "payments"}, requested); err != nil {
		t.Fatal(err)
	}
	if got := requested.Annotations[corev1alpha1.RollbackTargetRevisionAnnotation]; got != chosen {
		t.Fatalf("rollback-target-revision = %q, want the chosen %s", got, chosen)
	}

	// The spec changed since the Revisions were built: the rollback plans
	// another Revision of the commit, which the request does not approve.
	changed := manual()
	old := revisions(changed)
	changed.Spec.Source.Render.Type = corev1alpha1.RenderTypeKustomize
	got := run(stamping(rollbackClient(t, changed, old...)))
	if strings.Contains(got, "approves and deploys") || !strings.Contains(got, "spec changed since "+old[0].Name+" was built") || !strings.Contains(got, "ksync sync") {
		t.Fatalf("a changed spec claims to deploy the chosen Revision:\n%s", got)
	}

	// A requester but no desired state: the chosen Revision never deployed.
	undeployed := interceptor.NewClient(rollbackClient(t, manual(), revisions(manual())...).(client.WithWatch), interceptor.Funcs{
		Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
			annotations := obj.GetAnnotations()
			annotations[corev1alpha1.RollbackRequestedByAnnotation] = "alice@example.com"
			obj.SetAnnotations(annotations)
			return c.Update(ctx, obj, opts...)
		},
	})
	if got := run(undeployed); strings.Contains(got, "approves and deploys") || strings.Contains(got, "webhooks") || !strings.Contains(got, "never deployed") || !strings.Contains(got, "ksync sync") {
		t.Fatalf("a Revision that never deployed:\n%s", got)
	}

	if got := run(rollbackClient(t, manual(), revisions(manual())...)); !strings.Contains(got, "no requester was recorded") || !strings.Contains(got, "awaits approval") {
		t.Fatalf("without a recorded requester the output does not say the target awaits approval:\n%s", got)
	}

	automatic := manual()
	automatic.Spec.Sync.Automatic = true
	if got := run(rollbackClient(t, automatic, revisions(automatic)...)); !strings.Contains(got, "the request deploys "+chosen+"\n") || strings.Contains(got, "approv") {
		t.Fatalf("automatic sync output:\n%s", got)
	}
}

// rollbackFixture holds a manual-sync Application in namespace with two
// Healthy Revisions named as the controller names them, and a client whose
// updates carry the record the admission webhook would add, bound to the
// Application as bound.
func rollbackFixture(t *testing.T, namespace string, bound func(*corev1alpha1.Application)) (*corev1alpha1.Application, []corev1alpha1.Revision, func(*corev1alpha1.Application) client.Client) {
	t.Helper()
	app := rollbackApp("b-sha", "b-sha")
	app.Namespace = namespace
	app.Spec.Sync.Automatic = false
	app.Spec.Source.Path = "apps/payments"
	app.Status.ServiceAccountName = "payments-deployer"
	revisions := make([]corev1alpha1.Revision, 0, 2)
	for i, commit := range []string{"a-sha", "b-sha"} {
		name, _, err := revisionid.For(app, commit, app.Status.ServiceAccountName)
		if err != nil {
			t.Fatal(err)
		}
		rev := rollbackRevision(name, commit, corev1alpha1.RevisionPhaseHealthy, i)
		rev.Namespace = namespace
		rev.Spec.DesiredStateHash = "hash-" + commit
		rev.Status.DeployedDesiredStateHash = "hash-" + commit
		revisions = append(revisions, rev)
	}
	build := func(current *corev1alpha1.Application) client.Client {
		return interceptor.NewClient(rollbackClient(t, current, revisions...).(client.WithWatch), interceptor.Funcs{
			Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
				requested := obj.(*corev1alpha1.Application).DeepCopy()
				if bound != nil {
					bound(requested)
				}
				annotations := obj.GetAnnotations()
				annotations[corev1alpha1.RollbackRequestedByAnnotation] = "alice@example.com"
				annotations[corev1alpha1.RollbackTargetHashAnnotation] = revisionid.Binding("hash-a-sha", requested)
				obj.SetAnnotations(annotations)
				return c.Update(ctx, obj, opts...)
			},
		})
	}
	return app, revisions, build
}

// Catches the changed-spec message printing commands that do not run: it
// left out -n for a namespace other than default and --revision, which
// ksync sync requires.
func TestRollbackAfterASpecChangePrintsRunnableCommands(t *testing.T) {
	for _, namespace := range []string{"ksync-demo", "default"} {
		app, _, build := rollbackFixture(t, namespace, nil)
		app.Spec.Source.Render.Type = corev1alpha1.RenderTypeKustomize
		rebuilt, _, err := revisionid.For(app, "a-sha", app.Status.ServiceAccountName)
		if err != nil {
			t.Fatal(err)
		}
		var stdout bytes.Buffer
		if err := rollback(context.Background(), build(app), namespace, "payments", "", &stdout); err != nil {
			t.Fatal(err)
		}
		flag := ""
		if namespace != "default" {
			flag = " -n " + namespace
		}
		for _, want := range []string{
			"ksync plan payments" + flag + "\n",
			"ksync sync payments" + flag + " --revision " + rebuilt,
		} {
			if !strings.Contains(stdout.String(), want) {
				t.Errorf("%s: output lacks %q:\n%s", namespace, want, stdout.String())
			}
		}
	}
}

// Catches ksync rollback claiming a pending request approves its target
// after spec.sync or spec.strategy changed: the webhook keeps the request's
// record, bound to the policy it was made under, and the controller then
// refuses it.
func TestRollbackReportsARequestBoundToAnEarlierSyncPolicy(t *testing.T) {
	// The request was recorded while the Application did not prune.
	app, revisions, build := rollbackFixture(t, "default", func(a *corev1alpha1.Application) { a.Spec.Sync.Prune = false })
	app.Spec.Sync.Prune = true
	var stdout bytes.Buffer
	if err := rollback(context.Background(), build(app), "default", "payments", "", &stdout); err != nil {
		t.Fatal(err)
	}
	got := stdout.String()
	if strings.Contains(got, "approves and deploys") || !strings.Contains(got, "spec.sync or spec.strategy") || !strings.Contains(got, "ksync sync payments --revision "+revisions[0].Name) {
		t.Fatalf("a request bound to an earlier sync policy:\n%s", got)
	}

	// Recorded under the policy the Application has, the request approves.
	app, revisions, build = rollbackFixture(t, "default", nil)
	stdout.Reset()
	if err := rollback(context.Background(), build(app), "default", "payments", "", &stdout); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "the request approves and deploys "+revisions[0].Name) {
		t.Fatalf("a current request:\n%s", stdout.String())
	}
}
