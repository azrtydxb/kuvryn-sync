package planner

import (
	"cmp"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
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
	ordered := slices.SortedFunc(maps.Keys(ids), func(a, b resource.ID) int { return cmp.Compare(a.String(), b.String()) })

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
		if slices.Contains(owners[path], solderFieldManager) {
			paths[path] = struct{}{}
		}
	}
	out := []corev1alpha1.PlanFieldChange{}
	for _, path := range slices.Sorted(maps.Keys(paths)) {
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
		// Sorted, so a dotted key and a nested path that flatten alike
		// resolve the same way every time.
		for _, key := range slices.Sorted(maps.Keys(typed)) {
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
// Server-Side Apply would refuse without force. A field Solder shares with
// other managers conflicts with each of them.
func detectConflicts(live unstructured.Unstructured, fields []corev1alpha1.PlanFieldChange) []corev1alpha1.PlanConflict {
	owners := fieldOwners(live)
	conflicts := []corev1alpha1.PlanConflict{}
	for _, field := range fields {
		for _, manager := range owners[field.Path] {
			if manager == solderFieldManager {
				continue
			}
			conflicts = append(conflicts, corev1alpha1.PlanConflict{Path: field.Path, Manager: manager, Policy: corev1alpha1.ConflictPolicyFail})
		}
	}
	return conflicts
}

// ownership maps field paths, in the planner's flattened notation, to every
// field manager that owns them, sorted. List items, which managedFields
// identify by key (k:), value (v:), or index (i:), are resolved against the
// live object.
type ownership map[string][]string

func fieldOwners(live unstructured.Unstructured) ownership {
	o := ownership{}
	for _, managed := range live.GetManagedFields() {
		if managed.Manager == "" || managed.FieldsV1 == nil {
			continue
		}
		var tree map[string]any
		if err := json.Unmarshal(managed.FieldsV1.Raw, &tree); err != nil {
			continue
		}
		o.walk(tree, "", live.Object, managed.Manager)
	}
	for path, managers := range o {
		slices.Sort(managers)
		o[path] = slices.Compact(managers)
	}
	return o
}

func (o ownership) walk(node map[string]any, prefix string, liveValue any, manager string) {
	for key, value := range node {
		if key == "." {
			continue
		}
		child, _ := value.(map[string]any)
		path, childLive, ok := step(prefix, key, liveValue)
		if !ok {
			// The live object no longer has this entry; nothing to own.
			continue
		}
		if len(child) == 0 || (len(child) == 1 && child["."] != nil) {
			o[path] = append(o[path], manager)
			continue
		}
		o.walk(child, path, childLive, manager)
	}
}

// step resolves one managedFields key under prefix to a flattened path and
// the matching live value.
func step(prefix, key string, liveValue any) (string, any, bool) {
	if name, ok := strings.CutPrefix(key, "f:"); ok {
		path := name
		if prefix != "" {
			path = prefix + "." + name
		}
		liveMap, _ := liveValue.(map[string]any)
		return path, liveMap[name], true
	}
	items, _ := liveValue.([]any)
	match := func(i int) (string, any, bool) { return fmt.Sprintf("%s[%d]", prefix, i), items[i], true }
	switch {
	case strings.HasPrefix(key, "k:"):
		var fields map[string]any
		if err := json.Unmarshal([]byte(key[2:]), &fields); err != nil {
			return "", nil, false
		}
		for i, item := range items {
			entry, _ := item.(map[string]any)
			if entry != nil && matchesKey(entry, fields) {
				return match(i)
			}
		}
	case strings.HasPrefix(key, "v:"):
		var want any
		if err := json.Unmarshal([]byte(key[2:]), &want); err != nil {
			return "", nil, false
		}
		for i, item := range items {
			if fmt.Sprint(item) == fmt.Sprint(want) {
				return match(i)
			}
		}
	case strings.HasPrefix(key, "i:"):
		var index int
		if _, err := fmt.Sscanf(key[2:], "%d", &index); err == nil && index >= 0 && index < len(items) {
			return match(index)
		}
	}
	return "", nil, false
}

func matchesKey(entry, fields map[string]any) bool {
	for name, want := range fields {
		if fmt.Sprint(entry[name]) != fmt.Sprint(want) {
			return false
		}
	}
	return true
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
