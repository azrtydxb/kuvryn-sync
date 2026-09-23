package planner

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	corev1alpha1 "github.com/azrtydxb/solder/api/v1alpha1"
	"github.com/azrtydxb/solder/internal/normalize"
	"github.com/azrtydxb/solder/internal/redact"
	"github.com/azrtydxb/solder/internal/resource"
)

const solderFieldManager = "solder"

// Change describes one resource-level plan action.
type Change struct {
	ID          resource.ID
	Action      corev1alpha1.PlanAction
	Fields      []corev1alpha1.PlanFieldChange
	Conflicts   []corev1alpha1.PlanConflict
	Destructive bool
	Warnings    []string
}

// Plan is a deterministic resource-level change plan.
type Plan struct {
	Changes []Change
	Summary corev1alpha1.PlanSummary
}

// Build classifies desired and live objects into a deterministic plan.
func Build(desired []unstructured.Unstructured, live []unstructured.Unstructured) (Plan, error) {
	desiredByID := map[resource.ID]unstructured.Unstructured{}
	liveByID := map[resource.ID]unstructured.Unstructured{}
	for _, obj := range desired {
		id, err := resource.FromObject(obj)
		if err != nil {
			return Plan{}, err
		}
		desiredByID[id] = obj
	}
	for _, obj := range live {
		id, err := resource.FromObject(obj)
		if err != nil {
			return Plan{}, err
		}
		liveByID[id] = obj
	}
	ids := make(map[resource.ID]struct{}, len(desiredByID)+len(liveByID))
	for id := range desiredByID {
		ids[id] = struct{}{}
	}
	for id := range liveByID {
		ids[id] = struct{}{}
	}
	ordered := make([]resource.ID, 0, len(ids))
	for id := range ids {
		ordered = append(ordered, id)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].String() < ordered[j].String() })

	plan := Plan{Changes: make([]Change, 0, len(ordered))}
	for _, id := range ordered {
		desiredObj, want := desiredByID[id]
		liveObj, have := liveByID[id]
		change := Change{ID: id, Action: corev1alpha1.PlanActionUnchanged}
		switch {
		case want && !have:
			change.Action = corev1alpha1.PlanActionCreate
			change.Fields = createFieldChanges(desiredObj)
			plan.Summary.Create++
		case !want && have:
			change.Action = corev1alpha1.PlanActionDelete
			change.Destructive = true
			change.Warnings = deleteWarnings(liveObj)
			plan.Summary.Delete++
		case want && have:
			fields, err := changedFields(desiredObj, liveObj)
			if err != nil {
				return Plan{}, err
			}
			if len(fields) == 0 {
				plan.Summary.Unchanged++
			} else {
				change.Action = corev1alpha1.PlanActionUpdate
				change.Fields = fields
				change.Conflicts = detectConflicts(liveObj, fields)
				plan.Summary.Update++
			}
		}
		plan.Changes = append(plan.Changes, change)
	}
	return plan, nil
}

func createFieldChanges(obj unstructured.Unstructured) []corev1alpha1.PlanFieldChange {
	if isSecret(obj) {
		return secretFieldChanges(obj, unstructured.Unstructured{})
	}
	return nil
}

// changedFields lists what Server-Side Apply would change: every field Solder
// declares that differs from live, and fields Solder owned that desired state
// no longer declares. Fields only other managers own, or that the API server
// defaulted, are not Solder's and are never reported.
func changedFields(desired, live unstructured.Unstructured) ([]corev1alpha1.PlanFieldChange, error) {
	if isSecret(desired) || isSecret(live) {
		equal, err := normalize.Equal(desired, live)
		if err != nil || equal {
			return nil, err
		}
		return secretFieldChanges(desired, live), nil
	}
	owners := fieldOwners(live)
	d, err := normalize.Object(desired)
	if err != nil {
		return nil, err
	}
	l, err := normalize.Object(live)
	if err != nil {
		return nil, err
	}
	df := map[string]string{}
	lf := map[string]string{}
	flatten(df, "", d.Object)
	flatten(lf, "", l.Object)
	paths := make(map[string]struct{}, len(df)+len(lf))
	for path := range df {
		paths[path] = struct{}{}
	}
	for path := range lf {
		if _, declared := df[path]; declared {
			continue
		}
		if manager, owned := owners.owner(path); owned && manager == solderFieldManager {
			paths[path] = struct{}{}
		}
	}
	ordered := make([]string, 0, len(paths))
	for path := range paths {
		ordered = append(ordered, path)
	}
	sort.Strings(ordered)
	out := []corev1alpha1.PlanFieldChange{}
	for _, path := range ordered {
		if df[path] == lf[path] {
			continue
		}
		before := lf[path]
		after := df[path]
		redacted := redact.SensitivePath(path)
		if redacted {
			before = redact.Value(before)
			after = redact.Value(after)
		}
		out = append(out, corev1alpha1.PlanFieldChange{Path: path, Before: before, After: after, Redacted: redacted})
	}
	return out, nil
}

func flatten(out map[string]string, prefix string, v any) {
	switch typed := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			path := key
			if prefix != "" {
				path = prefix + "." + key
			}
			flatten(out, path, typed[key])
		}
	case []any:
		for i, item := range typed {
			flatten(out, fmt.Sprintf("%s[%d]", prefix, i), item)
		}
	default:
		out[prefix] = fmt.Sprint(typed)
	}
}

func isSecret(obj unstructured.Unstructured) bool {
	return obj.GetAPIVersion() == "v1" && obj.GetKind() == "Secret"
}

func secretFieldChanges(desired, live unstructured.Unstructured) []corev1alpha1.PlanFieldChange {
	paths := []string{"data", "stringData"}
	out := []corev1alpha1.PlanFieldChange{}
	for _, path := range paths {
		before := lenStringMap(live.Object[path])
		after := lenStringMap(desired.Object[path])
		if before == after && before == 0 {
			continue
		}
		out = append(out, corev1alpha1.PlanFieldChange{Path: path, Before: redactedCount(before), After: redactedCount(after), Redacted: true})
	}
	return out
}

func lenStringMap(v any) int {
	switch typed := v.(type) {
	case map[string]any:
		return len(typed)
	case map[string]string:
		return len(typed)
	case map[string][]byte:
		return len(typed)
	default:
		return 0
	}
}

func redactedCount(n int) string {
	return fmt.Sprintf("%d key(s) %s", n, redact.Value("secret"))
}

func deleteWarnings(obj unstructured.Unstructured) []string {
	warnings := []string{"delete action is destructive"}
	ann := obj.GetAnnotations()
	if ann["solder.io/prune"] == "disabled" {
		warnings = append(warnings, "prune disabled by solder.io/prune annotation")
	}
	switch obj.GetKind() {
	case "Namespace", "CustomResourceDefinition", "PersistentVolumeClaim", "PersistentVolume", "Secret":
		warnings = append(warnings, "high-risk prune candidate requires policy approval")
	}
	return warnings
}

// detectConflicts reports changed fields another field manager owns, which
// Server-Side Apply would refuse without force.
func detectConflicts(live unstructured.Unstructured, fields []corev1alpha1.PlanFieldChange) []corev1alpha1.PlanConflict {
	owners := fieldOwners(live)
	conflicts := []corev1alpha1.PlanConflict{}
	for _, field := range fields {
		manager, owned := owners.owner(field.Path)
		if !owned || manager == solderFieldManager {
			continue
		}
		conflicts = append(conflicts, corev1alpha1.PlanConflict{Path: field.Path, Manager: manager, Policy: corev1alpha1.ConflictPolicyFail})
	}
	return conflicts
}

// ownership maps field paths, in the planner's flattened notation, to the
// field manager that owns them.
type ownership struct {
	fields map[string]string
	// lists are owned element by element; any path inside counts as owned.
	lists map[string]string
}

func fieldOwners(live unstructured.Unstructured) ownership {
	o := ownership{fields: map[string]string{}, lists: map[string]string{}}
	for _, managed := range live.GetManagedFields() {
		if managed.Manager == "" || managed.FieldsV1 == nil {
			continue
		}
		var tree map[string]any
		if err := json.Unmarshal(managed.FieldsV1.Raw, &tree); err != nil {
			continue
		}
		o.walk(tree, "", managed.Manager)
	}
	return o
}

func (o ownership) walk(node map[string]any, prefix, manager string) {
	for key, value := range node {
		name, isField := strings.CutPrefix(key, "f:")
		if !isField {
			if key != "." {
				o.lists[prefix] = manager
			}
			continue
		}
		path := name
		if prefix != "" {
			path = prefix + "." + name
		}
		child, _ := value.(map[string]any)
		if len(child) == 0 {
			o.fields[path] = manager
			continue
		}
		o.walk(child, path, manager)
	}
}

func (o ownership) owner(path string) (string, bool) {
	if manager, ok := o.fields[path]; ok {
		return manager, true
	}
	for list, manager := range o.lists {
		if strings.HasPrefix(path, list+"[") || strings.HasPrefix(path, list+".") {
			return manager, true
		}
	}
	return "", false
}

// RevisionPlan converts a plan to the bounded API status representation.
func (p Plan) RevisionPlan(limit int) corev1alpha1.RevisionPlan {
	out := corev1alpha1.RevisionPlan{Summary: p.Summary}
	if limit <= 0 || limit > len(p.Changes) {
		limit = len(p.Changes)
	}
	for _, change := range p.Changes[:limit] {
		out.Resources = append(out.Resources, corev1alpha1.PlanResourceChange{
			Resource: corev1alpha1.ResourceRef{
				APIVersion: change.ID.APIVersion(),
				Kind:       change.ID.Kind,
				Namespace:  change.ID.Namespace,
				Name:       change.ID.Name,
			},
			Action:      change.Action,
			Changes:     change.Fields,
			Conflicts:   change.Conflicts,
			Destructive: change.Destructive,
			Warnings:    change.Warnings,
		})
	}
	out.Summary.Truncated = len(p.Changes) > limit
	return out
}
