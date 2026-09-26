package planoutput

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	corev1alpha1 "github.com/azrtydxb/kuvryn-sync/api/v1alpha1"
	"github.com/azrtydxb/kuvryn-sync/internal/redact"
	"sigs.k8s.io/yaml"
)

// Document is the machine-readable shape printed by the plan CLI.
type Document struct {
	Application string `json:"application,omitempty"`
	// Revision is the source commit the plan was rendered from.
	Revision string `json:"revision,omitempty"`
	// RevisionName names the Revision object, which ksync sync approves.
	RevisionName string                     `json:"revisionName,omitempty"`
	Phase        corev1alpha1.RevisionPhase `json:"phase,omitempty"`
	Plan         corev1alpha1.RevisionPlan  `json:"plan"`
}

// Write renders a plan document in text, json, or yaml form.
func Write(w io.Writer, doc Document, format string) error {
	doc = RedactDocument(doc)
	switch strings.ToLower(format) {
	case "", "text", "table":
		return writeText(w, doc)
	case "json":
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(doc)
	case "yaml", "yml":
		b, err := yaml.Marshal(doc)
		if err != nil {
			return err
		}
		_, err = w.Write(b)
		return err
	default:
		return fmt.Errorf("unsupported output format %q", format)
	}
}

// RedactDocument applies the same central redaction policy used for status and CLI output.
func RedactDocument(doc Document) Document {
	for i := range doc.Plan.Resources {
		res := &doc.Plan.Resources[i]
		for j := range res.Changes {
			change := &res.Changes[j]
			if res.Resource.Kind == "Secret" || change.Redacted || redact.SensitivePath(change.Path) {
				change.Before = redact.Value(change.Before)
				change.After = redact.Value(change.After)
				change.Redacted = true
			}
		}
	}
	return doc
}

func writeText(w io.Writer, doc Document) error {
	if _, err := fmt.Fprintf(w, "Application: %s\n", doc.Application); err != nil {
		return err
	}
	if doc.RevisionName != "" {
		if _, err := fmt.Fprintf(w, "Revision:    %s\n", doc.RevisionName); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(w, "Commit:      %s\n", doc.Revision); err != nil {
		return err
	}
	if doc.Phase != "" {
		if _, err := fmt.Fprintf(w, "Phase:       %s\n", doc.Phase); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	for _, res := range doc.Plan.Resources {
		prefix := actionPrefix(res.Action)
		if res.Destructive {
			prefix = "! DELETE"
		}
		if _, err := fmt.Fprintf(w, "%s %s/%s/%s\n", prefix, res.Resource.Kind, namespaceOrDash(res.Resource.Namespace), res.Resource.Name); err != nil {
			return err
		}
		for _, warning := range res.Warnings {
			if _, err := fmt.Fprintf(w, "    WARNING: %s\n", warning); err != nil {
				return err
			}
		}
		for _, conflict := range res.Conflicts {
			if _, err := fmt.Fprintf(w, "    CONFLICT: %s owned by %s policy=%s\n", conflict.Path, conflict.Manager, conflict.Policy); err != nil {
				return err
			}
		}
		for _, change := range res.Changes {
			if _, err := fmt.Fprintf(w, "    %s\n      %s -> %s\n", change.Path, emptyDash(change.Before), emptyDash(change.After)); err != nil {
				return err
			}
		}
		if len(res.Changes) > 0 || len(res.Conflicts) > 0 || len(res.Warnings) > 0 {
			if _, err := fmt.Fprintln(w); err != nil {
				return err
			}
		}
	}
	_, err := fmt.Fprintf(w, "%d changed\n%d created\n%d deleted\n%d unchanged\n", doc.Plan.Summary.Update, doc.Plan.Summary.Create, doc.Plan.Summary.Delete, doc.Plan.Summary.Unchanged)
	return err
}

func actionPrefix(action corev1alpha1.PlanAction) string {
	switch action {
	case corev1alpha1.PlanActionCreate:
		return "+"
	case corev1alpha1.PlanActionUpdate:
		return "~"
	case corev1alpha1.PlanActionDelete:
		return "-"
	default:
		return "="
	}
}

func namespaceOrDash(namespace string) string {
	if namespace == "" {
		return "-"
	}
	return namespace
}

func emptyDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}
