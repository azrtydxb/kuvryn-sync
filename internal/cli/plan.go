package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	corev1alpha1 "github.com/azrtydxb/kuvryn-sync/api/v1alpha1"
	"github.com/azrtydxb/kuvryn-sync/internal/planoutput"
	"github.com/azrtydxb/kuvryn-sync/internal/version"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/yaml"
)

// Usage lists the CLI commands.
const Usage = `Usage: ksync <command> [arguments] [-n namespace]

Read:
  ksync apps                                     List Applications
  ksync repos                                    List Repositories
  ksync repo get <repository>                    Show one Repository
  ksync get <application>                        Show one Application (alias: status)
  ksync history <application> [-o table|json]    List an Application's Revisions
  ksync revision <revision>                      Show one Revision
  ksync plan <application> [-f file] [-o text|json|yaml]
                                                 Show the newest Revision's plan
  ksync diagnose <application>                   Explain why an Application is not Healthy
  ksync graph <application> [-o json|dot]        Print the live resource graph
  ksync drift <application>                      Show sync state (alias of get)

Change:
  ksync sync <application> --revision <revision> Approve a Revision's plan (alias: approve)
  ksync rollback <application> [--revision <revision>]
                                                 Roll back to a healthy Revision
  ksync suspend <application>                    Stop reconciling an Application
  ksync resume <application>                     Resume reconciling an Application

Other:
  ksync console [flags]                          Serve the read-only web console
  ksync install                                  Print the install commands
  ksync version                                  Print the version
  ksync help                                     Print this help

Without a command, or with flags only, ksync starts the controller manager.
`

// Run executes the CLI subcommand, returning false when args should start the controller manager.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) (bool, int) {
	if len(args) == 0 {
		return false, 0
	}
	var err error
	switch args[0] {
	case "apps", "applications":
		err = runApplications(ctx, args[1:], stdout, stderr)
	case "console":
		err = runConsole(ctx, args[1:], stdout, stderr)
	case "install":
		err = runInstall(args[1:], stdout)
	case "repos", "repositories":
		err = runRepositories(ctx, args[1:], stdout, stderr)
	case "repo":
		err = runRepo(ctx, args[1:], stdout, stderr)
	case "status", "get":
		err = runGetApplication(ctx, args[1:], stdout, stderr)
	case "history":
		err = runHistory(ctx, args[1:], stdout, stderr)
	case "revision":
		err = runRevision(ctx, args[1:], stdout, stderr)
	case "plan":
		err = runPlan(ctx, args[1:], stdout, stderr)
	case "sync", "approve":
		err = runSync(ctx, args[1:], stdout, stderr)
	case "rollback":
		err = runRollback(ctx, args[1:], stdout, stderr)
	case "drift":
		err = runDrift(ctx, args[1:], stdout, stderr)
	case "diagnose":
		err = runDiagnose(ctx, args[1:], stdout, stderr)
	case "graph":
		err = runGraph(ctx, args[1:], stdout, stderr)
	case "suspend":
		err = runSuspend(ctx, args[1:], stdout, stderr, true)
	case "resume":
		err = runSuspend(ctx, args[1:], stdout, stderr, false)
	case "version":
		_, _ = fmt.Fprintln(stdout, "ksync", version.Version)
		return true, 0
	case "help":
		_, _ = fmt.Fprint(stdout, Usage)
		return true, 0
	default:
		if len(args[0]) > 0 && args[0][0] == '-' {
			return false, 0
		}
		_, _ = fmt.Fprintf(stderr, "unknown ksync command %q\n\n%s", args[0], Usage)
		return true, 1
	}
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return true, 1
	}
	return true, 0
}

func runInstall(args []string, stdout io.Writer) error {
	if len(args) > 0 {
		return fmt.Errorf("usage: ksync install")
	}
	_, _ = fmt.Fprint(stdout, renderInstall(version.Version))
	return nil
}

func runApplications(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs, namespace := newFlagSet("ksync apps", stderr)
	if err := fs.Parse(interspersedFlags(args)); err != nil {
		return err
	}
	c, err := clusterClient()
	if err != nil {
		return err
	}
	var list corev1alpha1.ApplicationList
	if err := c.List(ctx, &list, client.InNamespace(*namespace)); err != nil {
		return err
	}
	_, _ = fmt.Fprint(stdout, RenderApplications(list.Items))
	return nil
}

func runRepositories(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs, namespace := newFlagSet("ksync repos", stderr)
	if err := fs.Parse(interspersedFlags(args)); err != nil {
		return err
	}
	c, err := clusterClient()
	if err != nil {
		return err
	}
	var list corev1alpha1.RepositoryList
	if err := c.List(ctx, &list, client.InNamespace(*namespace)); err != nil {
		return err
	}
	_, _ = fmt.Fprint(stdout, RenderRepositories(list.Items))
	return nil
}

func runRepo(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 || args[0] != "get" {
		return fmt.Errorf("usage: ksync repo get <name> [-n namespace]")
	}
	fs, namespace := newFlagSet("ksync repo get", stderr)
	if err := fs.Parse(interspersedFlags(args[1:])); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: ksync repo get <name> [-n namespace]")
	}
	c, err := clusterClient()
	if err != nil {
		return err
	}
	repo := &corev1alpha1.Repository{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: *namespace, Name: fs.Arg(0)}, repo); err != nil {
		return err
	}
	_, _ = fmt.Fprint(stdout, RenderRepositories([]corev1alpha1.Repository{*repo}))
	return nil
}

func runGetApplication(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs, namespace := newFlagSet("ksync get", stderr)
	if err := fs.Parse(interspersedFlags(args)); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: ksync get <application> [-n namespace]")
	}
	c, err := clusterClient()
	if err != nil {
		return err
	}
	app := &corev1alpha1.Application{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: *namespace, Name: fs.Arg(0)}, app); err != nil {
		return err
	}
	_, _ = fmt.Fprint(stdout, RenderApplications([]corev1alpha1.Application{*app}))
	return nil
}

func runHistory(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs, namespace := newFlagSet("ksync history", stderr)
	output := fs.String("output", "table", "output format: table or json")
	fs.StringVar(output, "o", "table", "output format: table or json")
	if err := fs.Parse(interspersedFlags(args)); err != nil {
		return err
	}
	if fs.NArg() != 1 || (*output != "table" && *output != "json") {
		return fmt.Errorf("usage: ksync history <application> [-n namespace] [-o table|json]")
	}
	c, err := clusterClient()
	if err != nil {
		return err
	}
	var list corev1alpha1.RevisionList
	if err := c.List(ctx, &list, client.InNamespace(*namespace)); err != nil {
		return err
	}
	revisions := []corev1alpha1.Revision{}
	for _, rev := range list.Items {
		if rev.Spec.ApplicationRef.Name == fs.Arg(0) {
			revisions = append(revisions, rev)
		}
	}
	if *output == "json" {
		out, err := RenderHistoryJSON(revisions)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintln(stdout, out)
		return nil
	}
	_, _ = fmt.Fprint(stdout, RenderHistory(revisions))
	return nil
}

func runRevision(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs, namespace := newFlagSet("ksync revision", stderr)
	if err := fs.Parse(interspersedFlags(args)); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: ksync revision <name> [-n namespace]")
	}
	c, err := clusterClient()
	if err != nil {
		return err
	}
	rev := &corev1alpha1.Revision{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: *namespace, Name: fs.Arg(0)}, rev); err != nil {
		return err
	}
	_, _ = fmt.Fprint(stdout, RenderRevision(*rev))
	return nil
}

func runSync(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs, namespace := newFlagSet("ksync sync", stderr)
	revision := fs.String("revision", "", "exact Revision object name to approve")
	if err := fs.Parse(interspersedFlags(args)); err != nil {
		return err
	}
	if fs.NArg() != 1 || *revision == "" {
		return fmt.Errorf("usage: ksync sync <application> --revision <revision> [-n namespace]")
	}
	c, err := clusterClient()
	if err != nil {
		return err
	}
	return approve(ctx, c, *namespace, fs.Arg(0), *revision, stdout)
}

// approve approves the plan of the named Revision as it is now: it sends the
// plan digest shown to the approver with the approval, so the admission
// webhook rejects it if the plan changed in the meantime.
func approve(ctx context.Context, c client.Client, namespace, application, revisionName string, stdout io.Writer) error {
	app := &corev1alpha1.Application{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: application}, app); err != nil {
		return err
	}
	rev := &corev1alpha1.Revision{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: revisionName}, rev); err != nil {
		return err
	}
	if rev.Spec.ApplicationRef.Name != app.Name {
		return fmt.Errorf("revision %s belongs to application %q, not %q", rev.Name, rev.Spec.ApplicationRef.Name, app.Name)
	}
	if rolledBack(rev) {
		return fmt.Errorf("revision %s was replaced by a rollback; push a new commit, delete the Revision, or run ksync rollback --revision %s", rev.Name, rev.Name)
	}
	digest := rev.Status.Plan.Digest
	if digest == "" {
		return fmt.Errorf("revision %s has no plan to approve yet", rev.Name)
	}
	_, _ = fmt.Fprintf(stdout, "approving %s for %s with plan digest %s\n", rev.Name, app.Name, digest)
	// Both annotations are always sent. A computed merge patch would drop
	// approved-revision when it already holds this Revision, and the webhook
	// would then check the digest against whatever Revision the server holds.
	body, err := json.Marshal(map[string]any{"metadata": map[string]any{"annotations": map[string]string{
		corev1alpha1.ApprovedRevisionAnnotation: rev.Name,
		corev1alpha1.ApproveDigestAnnotation:    digest,
	}}})
	if err != nil {
		return err
	}
	if err := c.Patch(ctx, app, client.RawPatch(types.MergePatchType, body)); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(stdout, "approved %s for %s\n", rev.Name, app.Name)
	return nil
}

func runRollback(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs, namespace := newFlagSet("ksync rollback", stderr)
	revisionName := fs.String("revision", "", "Revision object to roll back to; defaults to the newest known-good one not desired or deployed")
	if err := fs.Parse(interspersedFlags(args)); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: ksync rollback <application> [--revision revision] [-n namespace]")
	}
	c, err := clusterClient()
	if err != nil {
		return err
	}
	return rollback(ctx, c, *namespace, fs.Arg(0), *revisionName, stdout)
}

// rollback requests a rollback of application to the named Revision, or by
// default to the newest known-good one. It records the source revision rolled
// back from, which the controller holds once the rollback completes.
func rollback(ctx context.Context, c client.Client, namespace, application, revisionName string, stdout io.Writer) error {
	app := &corev1alpha1.Application{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: application}, app); err != nil {
		return err
	}
	// The desired revision is what reconciling would deploy next, so it is
	// what the rollback must hold; the deployed one is replaced anyway and
	// only deploys again if it is also desired. While a rollback is pending,
	// the desired revision is already its target, so a new request keeps the
	// source that one recorded.
	from := app.Annotations[corev1alpha1.RollbackFromAnnotation]
	if app.Annotations[corev1alpha1.RollbackRevisionAnnotation] == "" || from == "" {
		from = app.Status.DesiredRevision
	}
	if from == "" {
		from = app.Status.DeployedRevision
	}
	if from == "" {
		return fmt.Errorf("application %q has not resolved a revision yet; nothing to roll back from", app.Name)
	}
	target := revisionName
	if target == "" {
		var list corev1alpha1.RevisionList
		if err := c.List(ctx, &list, client.InNamespace(namespace)); err != nil {
			return err
		}
		var err error
		if target, err = defaultRollbackTarget(app, list.Items); err != nil {
			return err
		}
	}
	rev := &corev1alpha1.Revision{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: target}, rev); err != nil {
		return err
	}
	if rev.Spec.ApplicationRef.Name != app.Name {
		return fmt.Errorf("revision %s belongs to application %q, not %q", rev.Name, rev.Spec.ApplicationRef.Name, app.Name)
	}
	if rev.Spec.Source.Revision == from && !rolledBack(rev) {
		return fmt.Errorf("revision %s is the desired revision %s; roll back to an earlier one", rev.Name, from)
	}
	metav1.SetMetaDataAnnotation(&app.ObjectMeta, corev1alpha1.RollbackRevisionAnnotation, rev.Spec.Source.Revision)
	metav1.SetMetaDataAnnotation(&app.ObjectMeta, corev1alpha1.RollbackFromAnnotation, from)
	metav1.SetMetaDataAnnotation(&app.ObjectMeta, corev1alpha1.RollbackKindAnnotation, corev1alpha1.RollbackKindManual)
	if err := c.Update(ctx, app); err != nil {
		return err
	}
	if rolledBack(rev) {
		_, _ = fmt.Fprintf(stdout, "revision %s was replaced by an earlier rollback; rolling back to it lifts that hold\n", rev.Name)
	}
	if from == rev.Spec.Source.Revision {
		_, _ = fmt.Fprintf(stdout, "rollback requested for %s to %s (%s)\n", app.Name, rev.Name, rev.Spec.Source.Revision)
		return nil
	}
	_, _ = fmt.Fprintf(stdout, "rollback requested for %s to %s (%s), holding %s once it completes\n", app.Name, rev.Name, rev.Spec.Source.Revision, from)
	return nil
}

// rolledBack reports a Revision a completed rollback replaced.
func rolledBack(rev *corev1alpha1.Revision) bool {
	return apimeta.IsStatusConditionTrue(rev.Status.Conditions, corev1alpha1.RolledBackCondition)
}

// defaultRollbackTarget names the newest known-good Revision of app, Healthy
// or deployed by an earlier rollback, whose source revision is neither the
// desired nor the deployed one: rolling back to either would change nothing.
// Revisions created in the same second are ordered by name.
func defaultRollbackTarget(app *corev1alpha1.Application, revisions []corev1alpha1.Revision) (string, error) {
	var newest *corev1alpha1.Revision
	for i := range revisions {
		rev := &revisions[i]
		if rev.Spec.ApplicationRef.Name != app.Name {
			continue
		}
		if rev.Status.Phase != corev1alpha1.RevisionPhaseHealthy && rev.Status.Phase != corev1alpha1.RevisionPhaseRolledBack {
			continue
		}
		if source := rev.Spec.Source.Revision; source == app.Status.DeployedRevision || source == app.Status.DesiredRevision {
			continue
		}
		if newest == nil || newer(rev.ObjectMeta, newest.ObjectMeta) {
			newest = rev
		}
	}
	if newest == nil {
		return "", fmt.Errorf("no Healthy or RolledBack Revision of application %q other than the desired and deployed revisions to roll back to", app.Name)
	}
	return newest.Name, nil
}

func runDrift(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	return runGetApplication(ctx, args, stdout, stderr)
}

func runDiagnose(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs, namespace := newFlagSet("ksync diagnose", stderr)
	if err := fs.Parse(interspersedFlags(args)); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: ksync diagnose <application> [-n namespace]")
	}
	c, err := clusterClient()
	if err != nil {
		return err
	}
	return diagnose(ctx, c, *namespace, fs.Arg(0), stdout)
}

// diagnose prints the causal chains recorded on an Application and the
// failure of its latest Revision.
func diagnose(ctx context.Context, c client.Client, namespace, application string, stdout io.Writer) error {
	app := &corev1alpha1.Application{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: application}, app); err != nil {
		return err
	}
	var failure *corev1alpha1.RevisionFailure
	rev, err := newestRevision(ctx, c, application, namespace)
	switch {
	case err == nil:
		failure = rev.Status.Failure
	case !errors.Is(err, errNoRevision):
		return err
	}
	_, _ = fmt.Fprint(stdout, RenderDiagnosis(*app, failure))
	return nil
}

func runSuspend(ctx context.Context, args []string, stdout, stderr io.Writer, suspend bool) error {
	fs, namespace := newFlagSet("ksync suspend", stderr)
	if err := fs.Parse(interspersedFlags(args)); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: ksync suspend|resume <application> [-n namespace]")
	}
	c, err := clusterClient()
	if err != nil {
		return err
	}
	app := &corev1alpha1.Application{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: *namespace, Name: fs.Arg(0)}, app); err != nil {
		return err
	}
	app.Spec.Suspend = suspend
	if err := c.Update(ctx, app); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(stdout, "updated %s suspend=%t\n", app.Name, suspend)
	return nil
}

func runPlan(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs, namespace := newFlagSet("ksync plan", stderr)
	format := fs.String("o", "text", "output format: text, json, yaml")
	file := fs.String("f", "", "read Revision YAML/JSON from file instead of the cluster")
	if err := fs.Parse(interspersedFlags(args)); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: ksync plan <application> [-n namespace] [-f revision.yaml] [-o text|json|yaml]")
	}
	application := fs.Arg(0)
	rev, err := loadRevision(ctx, application, *namespace, *file)
	if err != nil {
		return err
	}
	doc := planoutput.Document{
		Application:  application,
		Revision:     rev.Spec.Source.Revision,
		RevisionName: rev.Name,
		Phase:        rev.Status.Phase,
		Plan:         rev.Status.Plan,
	}
	if err := planoutput.Write(stdout, doc, *format); err != nil {
		return err
	}
	text := *format == "" || strings.EqualFold(*format, "text") || strings.EqualFold(*format, "table")
	if text && rev.Name != "" && rev.Status.Phase == corev1alpha1.RevisionPhaseAwaitingApproval {
		ns, given := *namespace, flagSet(fs, "n", "namespace")
		if !given && rev.Namespace != "" {
			ns = rev.Namespace
		}
		_, _ = fmt.Fprintf(stdout, "\nApprove with: %s\n", approveCommand(application, ns, given, rev.Name))
	}
	return nil
}

// approveCommand is the ksync sync command that approves revision. It names
// the namespace unless it is the default one the CLI assumes without -n.
func approveCommand(application, namespace string, namespaceGiven bool, revision string) string {
	command := "ksync sync " + application
	if namespaceGiven || namespace != defaultNamespace {
		command += " -n " + namespace
	}
	return command + " --revision " + revision
}

// flagSet reports whether any of the named flags was given.
func flagSet(fs *flag.FlagSet, names ...string) bool {
	given := false
	fs.Visit(func(f *flag.Flag) {
		for _, name := range names {
			if f.Name == name {
				given = true
			}
		}
	})
	return given
}

// defaultNamespace is the namespace commands use without -n.
const defaultNamespace = "default"

// newFlagSet returns the flags of the named command, starting with its
// -n/--namespace flag.
func newFlagSet(name string, stderr io.Writer) (*flag.FlagSet, *string) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	namespace := fs.String("n", defaultNamespace, "namespace")
	fs.StringVar(namespace, "namespace", defaultNamespace, "namespace")
	return fs, namespace
}

func interspersedFlags(args []string) []string {
	flags := []string{}
	positional := []string{}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "-f" || arg == "-o" || arg == "-n" || arg == "--namespace" || arg == "--revision" {
			flags = append(flags, arg)
			if i+1 < len(args) {
				i++
				flags = append(flags, args[i])
			}
			continue
		}
		positional = append(positional, arg)
	}
	return append(flags, positional...)
}

func clusterClient() (client.Client, error) {
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		return nil, err
	}
	if err := corev1alpha1.AddToScheme(scheme); err != nil {
		return nil, err
	}
	rest, err := config.GetConfig()
	if err != nil {
		return nil, err
	}
	return client.New(rest, client.Options{Scheme: scheme})
}

func loadRevision(ctx context.Context, application, namespace, file string) (*corev1alpha1.Revision, error) {
	if file != "" {
		b, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		var rev corev1alpha1.Revision
		if err := yaml.Unmarshal(b, &rev); err != nil {
			return nil, err
		}
		return &rev, nil
	}
	c, err := clusterClient()
	if err != nil {
		return nil, err
	}
	return newestRevision(ctx, c, application, namespace)
}

// newestRevision returns the most recently created Revision of application.
// errNoRevision reports an Application with no Revision yet.
var errNoRevision = errors.New("no Revision found")

func newestRevision(ctx context.Context, c client.Client, application, namespace string) (*corev1alpha1.Revision, error) {
	var list corev1alpha1.RevisionList
	if err := c.List(ctx, &list, client.InNamespace(namespace)); err != nil {
		return nil, err
	}
	var newest *corev1alpha1.Revision
	for i := range list.Items {
		rev := &list.Items[i]
		if rev.Spec.ApplicationRef.Name != application {
			continue
		}
		if newest == nil || newer(rev.ObjectMeta, newest.ObjectMeta) {
			newest = rev
		}
	}
	if newest == nil {
		return nil, fmt.Errorf("%w for application %q in namespace %q", errNoRevision, application, namespace)
	}
	return newest, nil
}

func newer(a, b metav1.ObjectMeta) bool {
	if a.CreationTimestamp.After(b.CreationTimestamp.Time) {
		return true
	}
	if a.CreationTimestamp.Equal(&b.CreationTimestamp) {
		return a.Name > b.Name
	}
	return false
}
