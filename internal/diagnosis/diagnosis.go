// Package diagnosis explains why managed objects are unhealthy. It walks the
// resource graph down from each unhealthy managed object to the leaf
// evidence that explains it, such as a container that cannot pull its image
// or a Secret that does not exist, and reports each root cause once with the
// chain of resources that leads to it.
package diagnosis

import (
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	corev1alpha1 "github.com/azrtydxb/solder/api/v1alpha1"
	"github.com/azrtydxb/solder/internal/graph"
	"github.com/azrtydxb/solder/internal/health"
	"github.com/azrtydxb/solder/internal/redact"
	"github.com/azrtydxb/solder/internal/resource"
)

const (
	// MaxCauses bounds the causes Build returns.
	MaxCauses = 10
	// MaxChain bounds how deep Build walks, and so the length of a chain.
	MaxChain = 10
	// MaxMessage bounds a cause's message, in bytes.
	MaxMessage = 512
)

// Cause is one root cause and how an unhealthy managed object reaches it.
type Cause struct {
	// Resource is the resource at the root of the failure.
	Resource resource.ID
	// Reason is a CamelCase word naming the failure.
	Reason string
	// Message is the redacted, bounded evidence.
	Message string
	// Chain runs from the unhealthy managed object to Resource, both included.
	Chain []resource.ID
	// Fallback is true when no evidence was found beyond the managed
	// object's own health verdict, such as during an ordinary rollout.
	Fallback bool
}

// Input is what Build diagnoses.
type Input struct {
	// Results are the health verdicts of the managed objects.
	Results []health.Result
	// Graph relates the managed objects to their live descendants and
	// dependencies.
	Graph graph.Graph
	// Objects are the live objects in the graph.
	Objects []unstructured.Unstructured
}

// Build returns, deterministically, at most MaxCauses root causes for the
// unhealthy managed objects in in.Results, Degraded ones first. A root cause
// that several objects share is reported once, with the first chain found.
// An unhealthy object without deeper evidence is its own root cause.
func Build(in Input) []Cause {
	d := diagnoser{graph: in.Graph, objects: map[string]unstructured.Unstructured{}}
	for _, obj := range in.Objects {
		if id, err := resource.FromObject(obj); err == nil {
			d.objects[key(id)] = obj
		}
	}
	unhealthy := slices.DeleteFunc(slices.Clone(in.Results), func(result health.Result) bool {
		return result.State == corev1alpha1.HealthStateHealthy
	})
	slices.SortStableFunc(unhealthy, func(a, b health.Result) int {
		if rank(a.State) != rank(b.State) {
			return rank(a.State) - rank(b.State)
		}
		return strings.Compare(a.Resource.String(), b.Resource.String())
	})
	out := []Cause{}
	seen := map[string]bool{}
	for _, result := range unhealthy {
		causes := d.visit(result.Resource, nil)
		if len(causes) == 0 {
			reason := result.Reason
			if reason == "" {
				reason = string(result.State)
			}
			causes = []Cause{{Resource: result.Resource, Reason: reason, Message: result.Message, Chain: []resource.ID{result.Resource}, Fallback: true}}
		}
		for _, cause := range causes {
			cause.Reason = camel(cause.Reason)
			id := key(cause.Resource) + "#" + cause.Reason
			if seen[id] {
				continue
			}
			seen[id] = true
			cause.Message = bound(redact.String(cause.Message))
			out = append(out, cause)
			if len(out) == MaxCauses {
				return out
			}
		}
	}
	return out
}

// Status converts causes to their API form.
func Status(causes []Cause) []corev1alpha1.DiagnosisCause {
	if len(causes) == 0 {
		return nil
	}
	out := make([]corev1alpha1.DiagnosisCause, 0, len(causes))
	for _, cause := range causes {
		chain := make([]corev1alpha1.ResourceRef, 0, len(cause.Chain))
		for _, id := range cause.Chain {
			chain = append(chain, Ref(id))
		}
		out = append(out, corev1alpha1.DiagnosisCause{Resource: Ref(cause.Resource), Reason: cause.Reason, Message: cause.Message, Chain: chain})
	}
	return out
}

// Ref converts a resource identity to its API form.
func Ref(id resource.ID) corev1alpha1.ResourceRef {
	return corev1alpha1.ResourceRef{APIVersion: id.APIVersion(), Kind: id.Kind, Namespace: id.Namespace, Name: id.Name}
}

func rank(state corev1alpha1.HealthState) int {
	switch state {
	case corev1alpha1.HealthStateDegraded:
		return 0
	case corev1alpha1.HealthStateProgressing:
		return 1
	default:
		return 2
	}
}

type diagnoser struct {
	graph   graph.Graph
	objects map[string]unstructured.Unstructured
}

// visit returns the root causes below id, reached along path.
func (d diagnoser) visit(id resource.ID, path []resource.ID) []Cause {
	for _, step := range path {
		if key(step) == key(id) {
			return nil
		}
	}
	if len(path) == MaxChain {
		return nil
	}
	path = append(slices.Clone(path), id)
	node, ok := d.graph.Node(id)
	if ok && node.Missing {
		return []Cause{{Resource: id, Reason: "Missing" + id.Kind, Message: fmt.Sprintf("%s %s does not exist", id.Kind, name(id)), Chain: path}}
	}
	obj, ok := d.objects[key(id)]
	if !ok {
		return nil
	}
	switch {
	case id.Group == "" && id.Kind == "Pod":
		return d.pod(id, obj, path)
	case id.Group == "" && id.Kind == "PersistentVolumeClaim":
		if causes := d.follow(id, path, graph.EdgeBinds); len(causes) > 0 {
			return causes
		}
		if phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase"); phase == "Pending" {
			message := fmt.Sprintf("PersistentVolumeClaim %s is Pending", id.Name)
			if class, _, _ := unstructured.NestedString(obj.Object, "spec", "storageClassName"); class != "" {
				message += " with storage class " + class
			}
			return []Cause{{Resource: id, Reason: "ClaimPending", Message: message, Chain: path}}
		}
		return nil
	case id.Group == "" && id.Kind == "Service":
		if causes := d.follow(id, path, graph.EdgeSelects); len(causes) > 0 {
			return causes
		}
		return d.endpoints(id, path)
	}
	// Owned objects are the most specific evidence, then what the object
	// needs, then its own status.
	if causes := d.follow(id, path, graph.EdgeOwns); len(causes) > 0 {
		return causes
	}
	if causes := d.follow(id, path, graph.EdgeUses, graph.EdgeMounts, graph.EdgeRunsAs, graph.EdgeRoutes, graph.EdgeScales, graph.EdgeSelects); len(causes) > 0 {
		return causes
	}
	return ownEvidence(id, obj, path)
}

// follow visits the targets of id's required edges of the given types.
func (d diagnoser) follow(id resource.ID, path []resource.ID, types ...graph.EdgeType) []Cause {
	out := []Cause{}
	for _, edge := range d.graph.Out(id) {
		if edge.Optional || !slices.Contains(types, edge.Type) {
			continue
		}
		if podSpecEdge(edge.Type) && pullOnly(d.objects[key(id)], edge.To) {
			// A workload's pull Secrets matter only through a Pod that
			// cannot pull.
			continue
		}
		out = append(out, d.visit(edge.To, path)...)
	}
	return out
}

// pod explains a Pod that is not running and ready: first by what it needs
// and does not exist, then by its own status.
func (d diagnoser) pod(id resource.ID, obj unstructured.Unstructured, path []resource.ID) []Cause {
	if obj.GetDeletionTimestamp() != nil || podHealthy(obj) {
		return nil
	}
	reason, message, evident := podEvidence(obj)
	pulling := reason == "ImagePullBackOff" || reason == "ErrImagePull"
	refs := map[string]graph.Reference{}
	if spec, ok := graph.PodSpec(obj); ok {
		for _, ref := range graph.PodReferences(spec) {
			refs[ref.Kind+"/"+ref.Name] = ref
		}
	}
	out := []Cause{}
	for _, edge := range d.graph.Out(id) {
		if edge.Optional || (edge.Type != graph.EdgeUses && edge.Type != graph.EdgeMounts) {
			continue
		}
		// A running Pod's ServiceAccount existed when it was created, and a
		// pull Secret matters only while pulling fails.
		if ref := refs[edge.To.Kind+"/"+edge.To.Name]; ref.PullOnly && !pulling {
			continue
		}
		for _, cause := range d.visit(edge.To, path) {
			if evident && len(cause.Chain) == len(path)+1 {
				cause.Message += fmt.Sprintf("; Pod %s: %s: %s", id.Name, reason, message)
			}
			out = append(out, cause)
		}
	}
	if len(out) > 0 {
		return out
	}
	if !evident {
		return nil
	}
	return []Cause{{Resource: id, Reason: reason, Message: message, Chain: path}}
}

// endpoints reports a Service whose EndpointSlices hold no ready endpoint.
// Without any EndpointSlice there is no evidence either way.
func (d diagnoser) endpoints(id resource.ID, path []resource.ID) []Cause {
	slicesSeen, ready := 0, 0
	for _, edge := range d.graph.Out(id) {
		if edge.Type != graph.EdgeEndpoints {
			continue
		}
		slice, ok := d.objects[key(edge.To)]
		if !ok {
			continue
		}
		slicesSeen++
		endpoints, _, _ := unstructured.NestedSlice(slice.Object, "endpoints")
		for _, endpoint := range endpoints {
			m, _ := endpoint.(map[string]any)
			// A nil ready condition means ready.
			if value, found, _ := unstructured.NestedBool(m, "conditions", "ready"); !found || value {
				ready++
			}
		}
	}
	if slicesSeen == 0 || ready > 0 {
		return nil
	}
	return []Cause{{Resource: id, Reason: "NoReadyEndpoints", Message: fmt.Sprintf("Service %s has no ready endpoints", id.Name), Chain: path}}
}

// ownEvidence reads failure from an object's own status conditions.
func ownEvidence(id resource.ID, obj unstructured.Unstructured, path []resource.ID) []Cause {
	conditions := statusConditions(obj)
	if condition, ok := conditions["Failed"]; ok && condition.status == "True" && id.Group == "batch" && id.Kind == "Job" {
		message := condition.message
		if condition.reason != "" {
			message = condition.reason + ": " + message
		}
		return []Cause{{Resource: id, Reason: "JobFailed", Message: "Job failed: " + message, Chain: path}}
	}
	if condition, ok := conditions["ReplicaFailure"]; ok && condition.status == "True" {
		reason := condition.reason
		if reason == "" {
			reason = "ReplicaFailure"
		}
		return []Cause{{Resource: id, Reason: reason, Message: condition.message, Chain: path}}
	}
	if condition, ok := conditions["Progressing"]; ok && condition.status == "False" && condition.reason == "ProgressDeadlineExceeded" {
		return []Cause{{Resource: id, Reason: condition.reason, Message: condition.message, Chain: path}}
	}
	return nil
}

func podHealthy(obj unstructured.Unstructured) bool {
	phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase")
	if phase == "Succeeded" {
		return true
	}
	if phase != "Running" {
		return false
	}
	statuses, _, _ := unstructured.NestedSlice(obj.Object, "status", "containerStatuses")
	for _, raw := range statuses {
		status, _ := raw.(map[string]any)
		if ready, _ := status["ready"].(bool); !ready {
			return false
		}
	}
	return true
}

// waitingIgnored are waiting reasons of a container that is starting normally.
var waitingIgnored = map[string]bool{"": true, "ContainerCreating": true, "PodInitializing": true}

// podEvidence returns why a Pod is not running: it cannot be scheduled, a
// container waits for a reason other than starting up, or it failed.
func podEvidence(obj unstructured.Unstructured) (string, string, bool) {
	if condition, ok := statusConditions(obj)["PodScheduled"]; ok && condition.status == "False" && condition.reason == "Unschedulable" {
		return "Unschedulable", condition.message, true
	}
	containers := append(containerStatuses(obj, "initContainerStatuses"), containerStatuses(obj, "containerStatuses")...)
	for _, container := range containers {
		name, _ := container["name"].(string)
		waiting, _, _ := unstructured.NestedMap(container, "state", "waiting")
		reason, _ := waiting["reason"].(string)
		if waitingIgnored[reason] {
			continue
		}
		message := fmt.Sprintf("container %s is waiting", name)
		if text, _ := waiting["message"].(string); text != "" {
			message += ": " + text
		}
		if terminated, found, _ := unstructured.NestedMap(container, "lastState", "terminated"); found {
			message += "; last terminated: " + termination(terminated)
		}
		return reason, message, true
	}
	phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase")
	if phase != "Failed" {
		return "", "", false
	}
	for _, container := range containers {
		name, _ := container["name"].(string)
		terminated, _, _ := unstructured.NestedMap(container, "state", "terminated")
		if code, _, _ := unstructured.NestedInt64(terminated, "exitCode"); code != 0 {
			reason, _ := terminated["reason"].(string)
			if reason == "" || reason == "Error" {
				reason = "ContainerFailed"
			}
			return reason, fmt.Sprintf("container %s terminated: %s", name, termination(terminated)), true
		}
	}
	reason, _, _ := unstructured.NestedString(obj.Object, "status", "reason")
	message, _, _ := unstructured.NestedString(obj.Object, "status", "message")
	if reason == "" {
		reason = "PodFailed"
	}
	return reason, message, true
}

// termination describes a terminated container state.
func termination(terminated map[string]any) string {
	reason, _ := terminated["reason"].(string)
	if reason == "" {
		reason = "Terminated"
	}
	code, _, _ := unstructured.NestedInt64(terminated, "exitCode")
	text := fmt.Sprintf("%s (exit code %d)", reason, code)
	if message, _ := terminated["message"].(string); message != "" {
		text += ": " + message
	}
	return text
}

func containerStatuses(obj unstructured.Unstructured, field string) []map[string]any {
	raw, _, _ := unstructured.NestedSlice(obj.Object, "status", field)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

type condition struct {
	status, reason, message string
}

func statusConditions(obj unstructured.Unstructured) map[string]condition {
	out := map[string]condition{}
	raw, _, _ := unstructured.NestedSlice(obj.Object, "status", "conditions")
	for _, item := range raw {
		entry, _ := item.(map[string]any)
		kind, _ := entry["type"].(string)
		status, _ := entry["status"].(string)
		reason, _ := entry["reason"].(string)
		message, _ := entry["message"].(string)
		if kind != "" {
			out[kind] = condition{status: status, reason: reason, message: message}
		}
	}
	return out
}

func podSpecEdge(kind graph.EdgeType) bool {
	return kind == graph.EdgeUses || kind == graph.EdgeMounts || kind == graph.EdgeRunsAs
}

// pullOnly reports whether a Pod-template object refers to target only as an
// imagePullSecret.
func pullOnly(obj unstructured.Unstructured, target resource.ID) bool {
	spec, ok := graph.PodSpec(obj)
	if !ok {
		return false
	}
	for _, ref := range graph.PodReferences(spec) {
		if ref.Kind == target.Kind && ref.Name == target.Name {
			return ref.PullOnly
		}
	}
	return false
}

func name(id resource.ID) string {
	if id.Namespace == "" {
		return id.Name
	}
	return id.Namespace + "/" + id.Name
}

func key(id resource.ID) string {
	return id.Group + "/" + id.Kind + "/" + id.Namespace + "/" + id.Name
}

// camel keeps the letters and digits of reason and starts it with an upper
// case letter, as the API requires.
func camel(reason string) string {
	var b strings.Builder
	for _, r := range reason {
		if r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)) {
			b.WriteRune(r)
		}
	}
	out := b.String()
	if out == "" || !unicode.IsLetter(rune(out[0])) {
		out = "Unhealthy" + out
	}
	if len(out) > 128 {
		out = out[:128]
	}
	return strings.ToUpper(out[:1]) + out[1:]
}

// bound truncates message to MaxMessage bytes on a rune boundary.
func bound(message string) string {
	if len(message) <= MaxMessage {
		return message
	}
	cut := MaxMessage - len("...")
	for cut > 0 && !utf8.RuneStart(message[cut]) {
		cut--
	}
	return message[:cut] + "..."
}
