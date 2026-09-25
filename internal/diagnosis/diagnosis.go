// Package diagnosis explains why managed objects are unhealthy. It walks the
// resource graph down from each unhealthy managed object to the leaf
// evidence that explains it, such as a container that cannot pull its image
// or a Secret that does not exist, and reports each root cause once with the
// chain of resources that leads to it.
package diagnosis

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	corev1alpha1 "github.com/azrtydxb/kuvryn-sync/api/v1alpha1"
	"github.com/azrtydxb/kuvryn-sync/internal/graph"
	"github.com/azrtydxb/kuvryn-sync/internal/health"
	"github.com/azrtydxb/kuvryn-sync/internal/redact"
	"github.com/azrtydxb/kuvryn-sync/internal/resource"
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
// An unhealthy object without deeper evidence is its own root cause, and so
// is every unhealthy object once ctx is done or the walk used up its budget.
func Build(ctx context.Context, in Input) []Cause {
	causes, _ := build(ctx, in, visitBudget)
	return causes
}

// visitBudget bounds the nodes one Build explains: at most MaxChain depths
// of every object Collect may read, times MaxCauses, which is far more than
// a real Application needs.
const visitBudget = MaxCauses * MaxChain * graph.CollectObjectLimit

// build is Build with an explicit budget; it also returns how many nodes it
// explained.
func build(ctx context.Context, in Input, budget int) ([]Cause, int) {
	d := &diagnoser{
		ctx: ctx, graph: in.Graph, objects: map[string]unstructured.Unstructured{},
		memo: map[memoKey][]Cause{}, active: map[string]bool{}, budget: budget,
	}
	for _, obj := range in.Objects {
		if id, err := resource.FromObject(obj); err == nil {
			d.objects[graph.Key(id)] = obj
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
		causes := d.visit(result.Resource, 0)
		if len(causes) == 0 {
			reason := result.Reason
			if reason == "" {
				reason = string(result.State)
			}
			message := result.Message
			if len(in.Graph.Unread) > 0 {
				// Say why the walk may have found nothing deeper.
				message = strings.TrimPrefix(message+"; not visible: "+strings.Join(in.Graph.Unread, "; "), "; ")
			}
			causes = []Cause{{Resource: result.Resource, Reason: reason, Message: message, Chain: []resource.ID{result.Resource}, Fallback: true}}
		}
		for _, cause := range causes {
			cause.Reason = camel(cause.Reason)
			id := graph.Key(cause.Resource) + "#" + cause.Reason
			if seen[id] {
				continue
			}
			seen[id] = true
			cause.Message = bound(redact.String(cause.Message))
			out = append(out, cause)
			if len(out) == MaxCauses {
				return out, d.visits
			}
		}
	}
	return out, d.visits
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
	ctx     context.Context
	graph   graph.Graph
	objects map[string]unstructured.Unstructured
	// memo holds the causes below each node at each depth, so a node many
	// paths reach is explained once rather than once per path.
	memo map[memoKey][]Cause
	// active holds the nodes on the current path, to cut cycles.
	active  map[string]bool
	visits  int
	budget  int
	stopped bool
}

type memoKey struct {
	node  string
	depth int
}

// visit returns the root causes below id, reached at depth, each with a
// chain that starts at id. It stops walking once ctx is done or the budget
// is used up.
func (d *diagnoser) visit(id resource.ID, depth int) []Cause {
	node := graph.Key(id)
	key := memoKey{node: node, depth: depth}
	if causes, ok := d.memo[key]; ok {
		return causes
	}
	if d.stopped || d.active[node] || depth == MaxChain {
		return nil
	}
	d.visits++
	if d.visits > d.budget || d.ctx.Err() != nil {
		d.stopped = true
		return nil
	}
	d.active[node] = true
	causes := distinct(d.explain(id, depth))
	delete(d.active, node)
	d.memo[key] = causes
	return causes
}

// explain returns the root causes below id, each with a chain that starts
// at id.
func (d *diagnoser) explain(id resource.ID, depth int) []Cause {
	if node, ok := d.graph.Node(id); ok && node.Missing {
		return leaf(id, "Missing"+id.Kind, fmt.Sprintf("%s %s does not exist", id.Kind, id.QualifiedName()))
	}
	obj, ok := d.objects[graph.Key(id)]
	if !ok {
		return nil
	}
	switch {
	case id.Group == "" && id.Kind == "Pod":
		return d.pod(id, obj, depth)
	case id.Group == "" && id.Kind == "PersistentVolumeClaim":
		if causes := d.follow(id, depth, graph.EdgeBinds); len(causes) > 0 {
			return causes
		}
		if phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase"); phase == "Pending" {
			message := fmt.Sprintf("PersistentVolumeClaim %s is Pending", id.Name)
			if class, _, _ := unstructured.NestedString(obj.Object, "spec", "storageClassName"); class != "" {
				message += " with storage class " + class
			}
			return leaf(id, "ClaimPending", message)
		}
		return nil
	case id.Group == "" && id.Kind == "Service":
		if causes := d.follow(id, depth, graph.EdgeSelects); len(causes) > 0 {
			return causes
		}
		return d.endpoints(id)
	}
	// Owned objects are the most specific evidence, then what the object
	// needs, then its own status.
	if causes := d.follow(id, depth, graph.EdgeOwns); len(causes) > 0 {
		return causes
	}
	if causes := d.follow(id, depth, graph.EdgeUses, graph.EdgeMounts, graph.EdgeRunsAs, graph.EdgeRoutes, graph.EdgeScales, graph.EdgeSelects); len(causes) > 0 {
		return causes
	}
	return ownEvidence(id, obj)
}

// follow visits the targets of id's required edges of the given types.
func (d *diagnoser) follow(id resource.ID, depth int, types ...graph.EdgeType) []Cause {
	out := []Cause{}
	for _, edge := range d.graph.Out(id) {
		if edge.Optional || !slices.Contains(types, edge.Type) {
			continue
		}
		if podSpecEdge(edge.Type) && pullOnly(d.objects[graph.Key(id)], edge.To) {
			// A workload's pull Secrets matter only through a Pod that
			// cannot pull.
			continue
		}
		out = append(out, below(id, d.visit(edge.To, depth+1))...)
	}
	return out
}

// pod explains a Pod that is not running and ready: first by what it needs
// and does not exist, then by its own status.
func (d *diagnoser) pod(id resource.ID, obj unstructured.Unstructured, depth int) []Cause {
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
		for _, cause := range d.visit(edge.To, depth+1) {
			if evident && len(cause.Chain) == 1 {
				cause.Message += fmt.Sprintf("; Pod %s: %s: %s", id.Name, reason, message)
			}
			out = append(out, below(id, []Cause{cause})...)
		}
	}
	if len(out) > 0 {
		return out
	}
	if !evident {
		return nil
	}
	return leaf(id, reason, message)
}

// leaf is a root cause at id.
func leaf(id resource.ID, reason, message string) []Cause {
	return []Cause{{Resource: id, Reason: reason, Message: message, Chain: []resource.ID{id}}}
}

// below returns causes with id prepended to copies of their chains.
func below(id resource.ID, causes []Cause) []Cause {
	out := make([]Cause, 0, len(causes))
	for _, cause := range causes {
		cause.Chain = append([]resource.ID{id}, cause.Chain...)
		out = append(out, cause)
	}
	return out
}

// distinct drops causes with a root and reason seen before and keeps at
// most MaxCauses, which is all Build can report.
func distinct(causes []Cause) []Cause {
	seen := map[string]bool{}
	out := make([]Cause, 0, min(len(causes), MaxCauses))
	for _, cause := range causes {
		id := graph.Key(cause.Resource) + "#" + cause.Reason
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, cause)
		if len(out) == MaxCauses {
			break
		}
	}
	return out
}

// endpoints reports a Service whose EndpointSlices hold no ready endpoint.
// Without any EndpointSlice there is no evidence either way.
func (d *diagnoser) endpoints(id resource.ID) []Cause {
	slicesSeen, ready := 0, 0
	for _, edge := range d.graph.Out(id) {
		if edge.Type != graph.EdgeEndpoints {
			continue
		}
		slice, ok := d.objects[graph.Key(edge.To)]
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
	return leaf(id, "NoReadyEndpoints", fmt.Sprintf("Service %s has no ready endpoints", id.Name))
}

// ownEvidence reads failure from an object's own status conditions.
func ownEvidence(id resource.ID, obj unstructured.Unstructured) []Cause {
	conditions := health.Conditions(obj)
	if condition, ok := conditions["Failed"]; ok && condition.Status == "True" && id.Group == "batch" && id.Kind == "Job" {
		message := condition.Message
		if condition.Reason != "" {
			message = condition.Reason + ": " + message
		}
		return leaf(id, "JobFailed", "Job failed: "+message)
	}
	if condition, ok := conditions["ReplicaFailure"]; ok && condition.Status == "True" {
		reason := condition.Reason
		if reason == "" {
			reason = "ReplicaFailure"
		}
		return leaf(id, reason, condition.Message)
	}
	if condition, ok := conditions["Progressing"]; ok && condition.Status == "False" && condition.Reason == "ProgressDeadlineExceeded" {
		return leaf(id, condition.Reason, condition.Message)
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
	if condition, ok := health.Conditions(obj)["PodScheduled"]; ok && condition.Status == "False" && condition.Reason == "Unschedulable" {
		return "Unschedulable", condition.Message, true
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

// termination describes a terminated container state by its reason and
// exit code only. Its message is whatever the container wrote to its
// termination log, or its log output with FallbackToLogsOnError, which must
// not reach status or Events.
func termination(terminated map[string]any) string {
	reason, _ := terminated["reason"].(string)
	if reason == "" {
		reason = "Terminated"
	}
	code, _, _ := unstructured.NestedInt64(terminated, "exitCode")
	return fmt.Sprintf("%s (exit code %d)", reason, code)
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
