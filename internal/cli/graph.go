package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1alpha1 "github.com/azrtydxb/solder/api/v1alpha1"
	"github.com/azrtydxb/solder/internal/applier"
	"github.com/azrtydxb/solder/internal/graph"
	"github.com/azrtydxb/solder/internal/redact"
)

// defaultManagedKinds are inventoried for Applications that have not
// recorded status.managedKinds yet.
var defaultManagedKinds = []corev1alpha1.ManagedKind{
	{APIVersion: "v1", Kind: "ConfigMap"},
	{APIVersion: "v1", Kind: "Secret"},
	{APIVersion: "v1", Kind: "Service"},
	{APIVersion: "apps/v1", Kind: "Deployment"},
	{APIVersion: "apps/v1", Kind: "StatefulSet"},
	{APIVersion: "apps/v1", Kind: "DaemonSet"},
}

func runGraph(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs, namespace := newFlagSet("solder graph", stderr)
	output := fs.String("o", "json", "output format: json or dot")
	if err := fs.Parse(interspersedFlags(args)); err != nil {
		return err
	}
	if fs.NArg() != 1 || (*output != "json" && *output != "dot") {
		return fmt.Errorf("usage: solder graph <application> [-n namespace] [-o json|dot]")
	}
	c, err := clusterClient()
	if err != nil {
		return err
	}
	return writeGraph(ctx, c, *namespace, fs.Arg(0), *output, stdout)
}

// writeGraph builds the resource graph of an Application from the live
// cluster, as the caller's own identity, and writes it as JSON or DOT.
func writeGraph(ctx context.Context, c client.Client, namespace, application, format string, stdout io.Writer) error {
	app := &corev1alpha1.Application{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: application}, app); err != nil {
		return err
	}
	managed, err := managedObjects(ctx, c, app)
	if err != nil {
		return err
	}
	destination := app.Spec.Destination.Namespace
	if destination == "" {
		destination = app.Namespace
	}
	g, _ := graph.Collect(ctx, c, destination, managed)
	if format == "dot" {
		_, _ = fmt.Fprint(stdout, g.DOT())
		return nil
	}
	raw, err := json.Marshal(g)
	if err != nil {
		return err
	}
	var indented bytes.Buffer
	if err := json.Indent(&indented, raw, "", "  "); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(stdout, indented.String())
	return nil
}

// managedObjects lists the objects labelled as managed by app, of every kind
// it last applied. Kinds the caller may not list are left out.
func managedObjects(ctx context.Context, c client.Client, app *corev1alpha1.Application) ([]unstructured.Unstructured, error) {
	kinds := app.Status.ManagedKinds
	if len(kinds) == 0 {
		kinds = defaultManagedKinds
	}
	namespace := app.Spec.Destination.Namespace
	if namespace == "" {
		namespace = app.Namespace
	}
	out := []unstructured.Unstructured{}
	for _, kind := range kinds {
		gv, err := schema.ParseGroupVersion(kind.APIVersion)
		if err != nil {
			continue
		}
		list := &unstructured.UnstructuredList{}
		list.SetGroupVersionKind(gv.WithKind(kind.Kind + "List"))
		if err := c.List(ctx, list, client.InNamespace(namespace), client.MatchingLabels{applier.ApplicationLabelKey: app.Name}); err != nil {
			if apimeta.IsNoMatchError(err) || apierrors.IsNotFound(err) || apierrors.IsForbidden(err) {
				continue
			}
			return nil, err
		}
		for _, item := range list.Items {
			owner := item.GetLabels()[applier.ApplicationNamespaceLabelKey]
			if owner == "" || owner == app.Namespace {
				out = append(out, item)
			}
		}
	}
	return out, nil
}

// RenderDiagnosis renders an Application's recorded diagnosis and the
// failure of its latest Revision, if any, for people to read.
func RenderDiagnosis(app corev1alpha1.Application, failure *corev1alpha1.RevisionFailure) string {
	var b bytes.Buffer
	_, _ = fmt.Fprintf(&b, "%s: health %s, sync %s\n", app.Name, orUnknown(string(app.Status.Health.State)), orUnknown(string(app.Status.Sync.State)))
	if failure != nil {
		_, _ = fmt.Fprintf(&b, "Failure: %s: %s\n", failure.Reason, redact.String(failure.Message))
	}
	if len(app.Status.Diagnosis) == 0 {
		if failure == nil {
			_, _ = fmt.Fprintln(&b, "No failure or unhealthy resource recorded")
		}
		return b.String()
	}
	_, _ = fmt.Fprintf(&b, "Causes (%d):\n", len(app.Status.Diagnosis))
	for i, cause := range app.Status.Diagnosis {
		_, _ = fmt.Fprintf(&b, "\n%d. %s  %s\n", i+1, cause.Reason, refText(cause.Resource))
		if cause.Message != "" {
			_, _ = fmt.Fprintf(&b, "   %s\n", redact.String(cause.Message))
		}
		for depth, ref := range cause.Chain {
			if depth == 0 {
				_, _ = fmt.Fprintf(&b, "   %s\n", refText(ref))
				continue
			}
			_, _ = fmt.Fprintf(&b, "   %s└─ %s\n", strings.Repeat("   ", depth-1), refText(ref))
		}
	}
	return b.String()
}

func refText(ref corev1alpha1.ResourceRef) string {
	if ref.Namespace == "" {
		return ref.Kind + "/" + ref.Name
	}
	return ref.Kind + "/" + ref.Namespace + "/" + ref.Name
}

func orUnknown(value string) string {
	if value == "" {
		return "Unknown"
	}
	return value
}
