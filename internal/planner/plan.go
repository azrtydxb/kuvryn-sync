package planner

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
			equal, err := normalize.Equal(desiredObj, liveObj)
			if err != nil {
				return Plan{}, err
			}
			if equal {
				plan.Summary.Unchanged++
			} else {
				change.Action = corev1alpha1.PlanActionUpdate
				fields, err := changedFields(desiredObj, liveObj)
				if err != nil {
					return Plan{}, err
				}
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

func changedFields(desired, live unstructured.Unstructured) ([]corev1alpha1.PlanFieldChange, error) {
	if isSecret(desired) || isSecret(live) {
		return secretFieldChanges(desired, live), nil
	}
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
		paths[path] = struct{}{}
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

func detectConflicts(live unstructured.Unstructured, fields []corev1alpha1.PlanFieldChange) []corev1alpha1.PlanConflict {
	if len(fields) == 0 {
		return nil
	}
	owned := map[string]string{}
	for _, managed := range live.GetManagedFields() {
		if managed.Manager == "" || managed.Manager == solderFieldManager || managed.Operation != metav1.ManagedFieldsOperationApply || managed.FieldsV1 == nil {
			continue
		}
		raw, err := managed.FieldsV1.MarshalJSON()
		if err != nil {
			continue
		}
		var tree map[string]any
		if err := json.Unmarshal(raw, &tree); err != nil {
			continue
		}
		for _, top := range topLevelManagedFields(tree) {
			owned[top] = managed.Manager
		}
	}
	conflicts := []corev1alpha1.PlanConflict{}
	seen := map[string]struct{}{}
	for _, field := range fields {
		top := strings.Split(strings.Split(field.Path, ".")[0], "[")[0]
		manager, ok := owned[top]
		if !ok {
			continue
		}
		key := field.Path + "\x00" + manager
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		conflicts = append(conflicts, corev1alpha1.PlanConflict{Path: field.Path, Manager: manager, Policy: corev1alpha1.ConflictPolicyFail})
	}
	return conflicts
}

func topLevelManagedFields(tree map[string]any) []string {
	fields := map[string]struct{}{}
	var walk func(map[string]any)
	walk = func(node map[string]any) {
		for key, value := range node {
			if strings.HasPrefix(key, "f:") {
				fields[strings.TrimPrefix(key, "f:")] = struct{}{}
			}
			child, ok := value.(map[string]any)
			if ok {
				walk(child)
			}
		}
	}
	walk(tree)
	out := make([]string, 0, len(fields))
	for field := range fields {
		out = append(out, field)
	}
	sort.Strings(out)
	return out
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
