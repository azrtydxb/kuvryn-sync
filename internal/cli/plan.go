package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	corev1alpha1 "github.com/azrtydxb/solder/api/v1alpha1"
	"github.com/azrtydxb/solder/internal/planoutput"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/yaml"
)

// Run executes the CLI subcommand, returning false when args should start the controller manager.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) (bool, int) {
	if len(args) == 0 {
		return false, 0
	}
	var err error
	switch args[0] {
	case "apps", "applications":
		err = runApplications(ctx, args[1:], stdout, stderr)
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
	case "suspend":
		err = runSuspend(ctx, args[1:], stdout, stderr, true)
	case "resume":
		err = runSuspend(ctx, args[1:], stdout, stderr, false)
	case "version":
		_, _ = fmt.Fprintln(stdout, "solder development")
		return true, 0
	default:
		if len(args[0]) > 0 && args[0][0] == '-' {
			return false, 0
		}
		_, _ = fmt.Fprintf(stderr, "unknown solder command %q\n", args[0])
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
		return fmt.Errorf("usage: solder install")
	}
	_, _ = fmt.Fprintln(stdout, "kubectl apply -f dist/install.yaml")
	_, _ = fmt.Fprintln(stdout, "# or: kubectl apply -k config/default")
	return nil
}

func runApplications(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("solder apps", flag.ContinueOnError)
	fs.SetOutput(stderr)
	namespace := fs.String("n", "default", "namespace")
	fs.StringVar(namespace, "namespace", "default", "namespace")
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
	fs := flag.NewFlagSet("solder repos", flag.ContinueOnError)
	fs.SetOutput(stderr)
	namespace := fs.String("n", "default", "namespace")
	fs.StringVar(namespace, "namespace", "default", "namespace")
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
		return fmt.Errorf("usage: solder repo get <name> [-n namespace]")
	}
	fs := flag.NewFlagSet("solder repo get", flag.ContinueOnError)
	fs.SetOutput(stderr)
	namespace := fs.String("n", "default", "namespace")
	fs.StringVar(namespace, "namespace", "default", "namespace")
	if err := fs.Parse(interspersedFlags(args[1:])); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: solder repo get <name> [-n namespace]")
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
	fs := flag.NewFlagSet("solder get", flag.ContinueOnError)
	fs.SetOutput(stderr)
	namespace := fs.String("n", "default", "namespace")
	fs.StringVar(namespace, "namespace", "default", "namespace")
	if err := fs.Parse(interspersedFlags(args)); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: solder get <application> [-n namespace]")
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
	fs := flag.NewFlagSet("solder history", flag.ContinueOnError)
	fs.SetOutput(stderr)
	namespace := fs.String("n", "default", "namespace")
	fs.StringVar(namespace, "namespace", "default", "namespace")
	output := fs.String("output", "table", "output format: table or json")
	fs.StringVar(output, "o", "table", "output format: table or json")
	if err := fs.Parse(interspersedFlags(args)); err != nil {
		return err
	}
	if fs.NArg() != 1 || (*output != "table" && *output != "json") {
		return fmt.Errorf("usage: solder history <application> [-n namespace] [-o table|json]")
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
	_, _ = fmt.Fprintln(stdout, "NAME\tPHASE\tREVISION\tAPPROVED BY")
	for _, rev := range revisions {
		approvedBy := ""
		if rev.Status.Approval != nil {
			approvedBy = rev.Status.Approval.ApprovedBy
		}
		_, _ = fmt.Fprintf(stdout, "%s\t%s\t%s\t%s\n", rev.Name, rev.Status.Phase, rev.Spec.Source.Revision, approvedBy)
	}
	return nil
}

func runRevision(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("solder revision", flag.ContinueOnError)
	fs.SetOutput(stderr)
	namespace := fs.String("n", "default", "namespace")
	fs.StringVar(namespace, "namespace", "default", "namespace")
	if err := fs.Parse(interspersedFlags(args)); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: solder revision <name> [-n namespace]")
	}
	c, err := clusterClient()
	if err != nil {
		return err
	}
	rev := &corev1alpha1.Revision{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: *namespace, Name: fs.Arg(0)}, rev); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(stdout, "NAME\tPHASE\tAPPLICATION\tREVISION\n%s\t%s\t%s\t%s\n", rev.Name, rev.Status.Phase, rev.Spec.ApplicationRef.Name, rev.Spec.Source.Revision)
	return nil
}

func runSync(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("solder sync", flag.ContinueOnError)
	fs.SetOutput(stderr)
	namespace := fs.String("n", "default", "namespace")
	fs.StringVar(namespace, "namespace", "default", "namespace")
	revision := fs.String("revision", "", "exact Revision object name to approve")
	if err := fs.Parse(interspersedFlags(args)); err != nil {
		return err
	}
	if fs.NArg() != 1 || *revision == "" {
		return fmt.Errorf("usage: solder sync <application> --revision <revision> [-n namespace]")
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
	digest := rev.Status.Plan.Digest
	if digest == "" {
		return fmt.Errorf("revision %s has no plan to approve yet", rev.Name)
	}
	_, _ = fmt.Fprintf(stdout, "approving %s for %s with plan digest %s\n", rev.Name, app.Name, digest)
	patch, err := BuildSyncPatch(*app, rev.Name, rev.Name, digest)
	if err != nil {
		return err
	}
	body, err := yaml.YAMLToJSON([]byte(patch.Patch))
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
	fs := flag.NewFlagSet("solder rollback", flag.ContinueOnError)
	fs.SetOutput(stderr)
	namespace := fs.String("n", "default", "namespace")
	fs.StringVar(namespace, "namespace", "default", "namespace")
	revisionName := fs.String("revision", "", "Revision object to roll back to; defaults to latest healthy")
	if err := fs.Parse(interspersedFlags(args)); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: solder rollback <application> [--revision revision] [-n namespace]")
	}
	c, err := clusterClient()
	if err != nil {
		return err
	}
	app := &corev1alpha1.Application{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: *namespace, Name: fs.Arg(0)}, app); err != nil {
		return err
	}
	target := *revisionName
	if target == "" {
		var list corev1alpha1.RevisionList
		if err := c.List(ctx, &list, client.InNamespace(*namespace)); err != nil {
			return err
		}
		var newest *corev1alpha1.Revision
		for i := range list.Items {
			rev := &list.Items[i]
			if rev.Spec.ApplicationRef.Name != app.Name || rev.Status.Phase != corev1alpha1.RevisionPhaseHealthy {
				continue
			}
			if newest == nil || newer(rev.ObjectMeta, newest.ObjectMeta) {
				newest = rev
			}
		}
		if newest != nil {
			target = newest.Name
		}
	}
	if target == "" {
		return fmt.Errorf("no healthy Revision found for application %q", app.Name)
	}
	rev := &corev1alpha1.Revision{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: *namespace, Name: target}, rev); err != nil {
		return err
	}
	ann := app.GetAnnotations()
	if ann == nil {
		ann = map[string]string{}
	}
	ann["solder.io/rollback-revision"] = rev.Spec.Source.Revision
	app.SetAnnotations(ann)
	if err := c.Update(ctx, app); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(stdout, "rollback requested for %s to %s (%s)\n", app.Name, rev.Name, rev.Spec.Source.Revision)
	return nil
}

func runDrift(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	return runGetApplication(ctx, args, stdout, stderr)
}

func runDiagnose(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("solder diagnose", flag.ContinueOnError)
	fs.SetOutput(stderr)
	namespace := fs.String("n", "default", "namespace")
	fs.StringVar(namespace, "namespace", "default", "namespace")
	if err := fs.Parse(interspersedFlags(args)); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: solder diagnose <application> [-n namespace]")
	}
	rev, err := loadRevision(ctx, fs.Arg(0), *namespace, "")
	if err != nil {
		return err
	}
	if rev.Status.Failure == nil {
		_, _ = fmt.Fprintf(stdout, "%s: no failure recorded\n", fs.Arg(0))
		return nil
	}
	_, _ = fmt.Fprintf(stdout, "%s: %s: %s\n", fs.Arg(0), rev.Status.Failure.Reason, rev.Status.Failure.Message)
	return nil
}

func runSuspend(ctx context.Context, args []string, stdout, stderr io.Writer, suspend bool) error {
	fs := flag.NewFlagSet("solder suspend", flag.ContinueOnError)
	fs.SetOutput(stderr)
	namespace := fs.String("n", "default", "namespace")
	fs.StringVar(namespace, "namespace", "default", "namespace")
	if err := fs.Parse(interspersedFlags(args)); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: solder suspend|resume <application> [-n namespace]")
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
	fs := flag.NewFlagSet("solder plan", flag.ContinueOnError)
	fs.SetOutput(stderr)
	format := fs.String("o", "text", "output format: text, json, yaml")
	file := fs.String("f", "", "read Revision YAML/JSON from file instead of the cluster")
	namespace := fs.String("n", "default", "namespace for cluster lookup")
	fs.StringVar(namespace, "namespace", "default", "namespace for cluster lookup")
	if err := fs.Parse(interspersedFlags(args)); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: solder plan <application> [-n namespace] [-f revision.yaml] [-o text|json|yaml]")
	}
	application := fs.Arg(0)
	rev, err := loadRevision(ctx, application, *namespace, *file)
	if err != nil {
		return err
	}
	doc := planoutput.Document{Application: application, Revision: rev.Spec.Source.Revision, Plan: rev.Status.Plan}
	return planoutput.Write(stdout, doc, *format)
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
		return nil, fmt.Errorf("no Revision found for application %q in namespace %q", application, namespace)
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
