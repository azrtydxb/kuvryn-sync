package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1alpha1 "github.com/azrtydxb/kuvryn-sync/api/v1alpha1"
	"github.com/azrtydxb/kuvryn-sync/internal/applier"
	"github.com/azrtydxb/kuvryn-sync/internal/graph"
	"github.com/azrtydxb/kuvryn-sync/internal/redact"
	"github.com/azrtydxb/kuvryn-sync/internal/resource"
)

func runGraph(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs, namespace := newFlagSet("ksync graph", stderr)
	output := fs.String("o", "text", "output format: text, json or dot")
	if err := fs.Parse(interspersedFlags(args)); err != nil {
		return err
	}
	if fs.NArg() != 1 || (*output != "text" && *output != "json" && *output != "dot") {
		return fmt.Errorf("usage: ksync graph <application> [-n namespace] [-o text|json|dot]")
	}
	c, err := newClient()
	if err != nil {
		return err
	}
	return writeGraph(ctx, c, *namespace, fs.Arg(0), *output, stdout)
}

// newClient connects to the cluster; tests replace it.
var newClient = clusterClient

// RenderGraph renders the resource graph of an Application as a text tree
// for people to read: each managed resource, then what it leads to along
// the graph's edges, each line naming the edge type. A missing object is
// marked missing, one that could not be checked unreadable, and an optional
// reference optional. A managed resource another one leads to is marked
// managed, since it has its own tree, and any other object reached again is
// marked shown above rather than expanded twice. Objects no managed
// resource leads to, and reads that failed, are listed after the tree.
func RenderGraph(application string, g graph.Graph, managed []resource.ID) string {
	t := graphTree{g: g, isRoot: map[string]bool{}, shown: map[string]bool{}}
	for _, node := range g.Nodes {
		for _, id := range managed {
			if key := graph.Key(id); key == graph.Key(node.ID) && !t.isRoot[key] {
				t.isRoot[key], t.shown[key] = true, true
				t.roots = append(t.roots, node.ID)
			}
		}
	}
	if len(t.roots) == 0 {
		_, _ = fmt.Fprintf(&t.b, "%s: no managed resources found\n", application)
	} else {
		t.writeHeader(application)
		for _, root := range t.roots {
			node, _ := g.Node(root)
			_, _ = fmt.Fprintf(&t.b, "%s%s\n", idText(root), markText(nodeMarks(node)))
			t.walk(root, "")
		}
		t.writeUnlinked()
	}
	writeUnread(&t.b, g.Unread)
	return t.b.String()
}

// graphTree writes a Graph as a text tree from its roots.
type graphTree struct {
	b      strings.Builder
	g      graph.Graph
	roots  []resource.ID
	isRoot map[string]bool
	// shown holds the objects already printed with what they lead to.
	shown map[string]bool
}

func (t *graphTree) writeHeader(application string) {
	missing, unreadable := 0, 0
	for _, node := range t.g.Nodes {
		if node.Missing {
			missing++
		}
		if node.Unreadable {
			unreadable++
		}
	}
	_, _ = fmt.Fprintf(&t.b, "%s: %s, %s", application, plural(len(t.roots), "managed resource"), plural(len(t.g.Nodes), "object"))
	if missing > 0 {
		_, _ = fmt.Fprintf(&t.b, ", %d missing", missing)
	}
	if unreadable > 0 {
		_, _ = fmt.Fprintf(&t.b, ", %d unreadable", unreadable)
	}
	t.b.WriteString("\n")
}

// walk writes what id leads to, each line under prefix.
func (t *graphTree) walk(id resource.ID, prefix string) {
	edges := t.g.Out(id)
	for i, edge := range edges {
		branch, indent := "├─ ", "│  "
		if i == len(edges)-1 {
			branch, indent = "└─ ", "   "
		}
		node, _ := t.g.Node(edge.To)
		marks := nodeMarks(node)
		if edge.Optional {
			marks = append(marks, "optional")
		}
		key := graph.Key(edge.To)
		expand := !t.shown[key]
		switch {
		case t.isRoot[key]:
			// Listed as its own tree, above or below.
			marks = append(marks, "managed")
		case !expand && len(t.g.Out(edge.To)) > 0:
			marks = append(marks, "shown above")
		}
		_, _ = fmt.Fprintf(&t.b, "%s%s%s %s%s\n", prefix, branch, edge.Type, idText(edge.To), markText(marks))
		if expand {
			t.shown[key] = true
			t.walk(edge.To, prefix+indent)
		}
	}
}

// writeUnlinked lists the objects no root leads to.
func (t *graphTree) writeUnlinked() {
	header := false
	for _, node := range t.g.Nodes {
		if t.shown[graph.Key(node.ID)] {
			continue
		}
		if !header {
			t.b.WriteString("\nNot linked to a managed resource:\n")
			header = true
		}
		t.b.WriteString(idText(node.ID) + markText(nodeMarks(node)) + "\n")
	}
}

func writeUnread(b *strings.Builder, unread []string) {
	if len(unread) == 0 {
		return
	}
	b.WriteString("\nCould not read:\n")
	for _, failure := range unread {
		b.WriteString(redact.String(strings.ReplaceAll(failure, "\n", " ")) + "\n")
	}
}

func nodeMarks(node graph.Node) []string {
	switch {
	case node.Missing:
		return []string{"missing"}
	case node.Unreadable:
		return []string{"unreadable"}
	}
	return nil
}

func markText(marks []string) string {
	if len(marks) == 0 {
		return ""
	}
	return "  (" + strings.Join(marks, ", ") + ")"
}

// idText names a resource as Kind/namespace/name, like the diagnosis chains.
func idText(id resource.ID) string {
	return refText(corev1alpha1.ResourceRef{Kind: id.Kind, Namespace: id.Namespace, Name: id.Name})
}

func plural(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// writeGraph builds the resource graph of an Application from the live
// cluster, as the caller's own identity, and writes it as a text tree, JSON
// or DOT.
func writeGraph(ctx context.Context, c client.Client, namespace, application, format string, stdout io.Writer) error {
	app := &corev1alpha1.Application{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: application}, app); err != nil {
		return err
	}
	// Secrets and ConfigMaps are only nodes of the graph: their identity is
	// enough, and their data is never read.
	managed, _, err := applier.ListManaged(ctx, c, app, applier.ListOptions{MetadataOnly: graph.IdentityOnly})
	if err != nil {
		return err
	}
	g, _ := graph.Collect(ctx, c, app.DestinationNamespace(), managed)
	switch format {
	case "dot":
		_, _ = fmt.Fprint(stdout, g.DOT())
		return nil
	case "text":
		ids := make([]resource.ID, 0, len(managed))
		for _, obj := range managed {
			if id, err := resource.FromObject(obj); err == nil {
				ids = append(ids, id)
			}
		}
		_, _ = fmt.Fprint(stdout, RenderGraph(app.Name, g, ids))
		return nil
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(g)
}

// RenderDiagnosis renders an Application's recorded diagnosis and the
// failure of its latest Revision, if any, for people to read.
func RenderDiagnosis(app corev1alpha1.Application, failure *corev1alpha1.RevisionFailure) string {
	var b bytes.Buffer
	_, _ = fmt.Fprintf(&b, "%s: health %s, sync %s\n", app.Name, orUnknown(string(app.Status.Health.State)), orUnknown(string(app.Status.Sync.State)))
	ready := apimeta.FindStatusCondition(app.Status.Conditions, "Ready")
	notReady := ready != nil && ready.Status == metav1.ConditionFalse
	// A rollout failure sets Ready to the same reason and message as the
	// Revision failure printed next, so it is shown once.
	repeatsFailure := notReady && failure != nil && ready.Reason == failure.Reason && ready.Message == failure.Message
	if notReady && !repeatsFailure {
		_, _ = fmt.Fprintf(&b, "Ready: False: %s: %s\n", ready.Reason, redact.String(ready.Message))
	}
	if failure != nil {
		_, _ = fmt.Fprintf(&b, "Failure: %s: %s\n", failure.Reason, redact.String(failure.Message))
	}
	if len(app.Status.Diagnosis) == 0 {
		if failure == nil && !notReady {
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
