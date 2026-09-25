// Package graph infers the relationships between live Kubernetes objects:
// ownership, selection, routing, scaling, storage, configuration and identity.
// The graph is deterministic, never fails on kinds it does not know, and
// records every object an object needs but the input does not hold as a
// missing node, so diagnosis can blame it.
package graph

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/azrtydxb/kuvryn-sync/internal/resource"
)

// EdgeType names an inferred Kubernetes relationship. Every edge points from
// the object that depends on, owns or targets another toward that object.
type EdgeType string

const (
	// EdgeOwns links an owner to an object whose ownerReferences name it.
	EdgeOwns EdgeType = "Owns"
	// EdgeSelects links a Service or PodDisruptionBudget to the Pods and
	// workloads its label selector matches.
	EdgeSelects EdgeType = "Selects"
	// EdgeEndpoints links a Service to its EndpointSlices.
	EdgeEndpoints EdgeType = "Endpoints"
	// EdgeRoutes links an Ingress to a backend Service.
	EdgeRoutes EdgeType = "Routes"
	// EdgeScales links a HorizontalPodAutoscaler to its scale target.
	EdgeScales EdgeType = "Scales"
	// EdgeBinds links a PersistentVolumeClaim to its PersistentVolume.
	EdgeBinds EdgeType = "Binds"
	// EdgeMounts links a Pod or workload to a PersistentVolumeClaim.
	EdgeMounts EdgeType = "Mounts"
	// EdgeUses links a Pod or workload to a ConfigMap or Secret.
	EdgeUses EdgeType = "Uses"
	// EdgeRunsAs links a Pod or workload to its ServiceAccount.
	EdgeRunsAs EdgeType = "RunsAs"
)

// Edge is a deterministic directed relationship between resources.
type Edge struct {
	From resource.ID
	To   resource.ID
	Type EdgeType
	// Optional is true when every reference behind the edge is marked
	// optional, so a missing target does not stop the source from running.
	Optional bool
}

// Node is one resource in the graph.
type Node struct {
	ID resource.ID
	// Missing marks an object that is referenced but was not in the input.
	Missing bool
	// Unreadable marks a referenced object whose existence could not be
	// checked, for example because reading it was forbidden.
	Unreadable bool
}

// Graph holds sorted nodes and edges.
type Graph struct {
	Nodes []Node
	Edges []Edge
	// Unread lists, sorted, the reads Collect could not make, such as
	// "could not list Pods: forbidden": the graph may be missing what they
	// would have found.
	Unread []string

	index map[string]int
	out   map[string][]Edge
}

// Build infers the common built-in relationships between objects. Objects
// without a valid identity are skipped, and kinds Build does not know only
// contribute their ownerReferences.
func Build(objects []unstructured.Unstructured) Graph {
	b := builder{nodes: map[string]*Node{}, objects: map[string]unstructured.Unstructured{}, edges: map[string]*Edge{}}
	for _, obj := range objects {
		id, err := resource.FromObject(obj)
		if err != nil {
			continue
		}
		key := Key(id)
		if _, seen := b.objects[key]; seen {
			continue
		}
		b.objects[key] = obj
		b.nodes[key] = &Node{ID: id}
	}
	keys := make([]string, 0, len(b.objects))
	for key := range b.objects {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		b.infer(b.nodes[key].ID, b.objects[key], keys)
	}
	return b.graph()
}

// Node returns the node for id, matched by group, kind, namespace and name.
func (g Graph) Node(id resource.ID) (Node, bool) {
	i, ok := g.index[Key(id)]
	if !ok {
		return Node{}, false
	}
	return g.Nodes[i], true
}

// Out returns the edges leaving id, in graph order.
func (g Graph) Out(id resource.ID) []Edge {
	return g.out[Key(id)]
}

// Missing returns the referenced objects that were not in the input.
func (g Graph) Missing() []resource.ID {
	out := []resource.ID{}
	for _, node := range g.Nodes {
		if node.Missing {
			out = append(out, node.ID)
		}
	}
	return out
}

// MarkUnreadable records that the existence of the missing objects ids could
// not be checked, so they are not reported as missing.
func (g *Graph) MarkUnreadable(ids ...resource.ID) {
	for _, id := range ids {
		i, ok := g.index[Key(id)]
		if !ok || !g.Nodes[i].Missing {
			continue
		}
		g.Nodes[i].Missing, g.Nodes[i].Unreadable = false, true
	}
}

// jsonRef is the serialized identity of a node.
type jsonRef struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Namespace  string `json:"namespace,omitempty"`
	Name       string `json:"name"`
}

type jsonNode struct {
	ID string `json:"id"`
	jsonRef
	Missing    bool `json:"missing,omitempty"`
	Unreadable bool `json:"unreadable,omitempty"`
}

type jsonEdge struct {
	From     string   `json:"from"`
	To       string   `json:"to"`
	Type     EdgeType `json:"type"`
	Optional bool     `json:"optional,omitempty"`
}

// MarshalJSON renders the graph as sorted nodes and edges that refer to nodes
// by their resource ID string.
func (g Graph) MarshalJSON() ([]byte, error) {
	out := struct {
		Nodes  []jsonNode `json:"nodes"`
		Edges  []jsonEdge `json:"edges"`
		Unread []string   `json:"unread,omitempty"`
	}{Nodes: make([]jsonNode, 0, len(g.Nodes)), Edges: make([]jsonEdge, 0, len(g.Edges)), Unread: g.Unread}
	for _, node := range g.Nodes {
		out.Nodes = append(out.Nodes, jsonNode{
			ID:         node.ID.String(),
			jsonRef:    jsonRef{APIVersion: node.ID.APIVersion(), Kind: node.ID.Kind, Namespace: node.ID.Namespace, Name: node.ID.Name},
			Missing:    node.Missing,
			Unreadable: node.Unreadable,
		})
	}
	for _, edge := range g.Edges {
		out.Edges = append(out.Edges, jsonEdge{From: edge.From.String(), To: edge.To.String(), Type: edge.Type, Optional: edge.Optional})
	}
	return json.Marshal(out)
}

// DOT renders the graph in Graphviz DOT. Missing nodes are dashed and red,
// unreadable nodes dotted and grey, and optional references dashed.
func (g Graph) DOT() string {
	var b bytes.Buffer
	b.WriteString("digraph kuvryn_sync {\n")
	for _, unread := range g.Unread {
		_, _ = fmt.Fprintf(&b, "  // %s\n", strings.ReplaceAll(unread, "\n", " "))
	}
	b.WriteString("  rankdir=LR;\n")
	b.WriteString("  node [shape=box, fontname=\"Helvetica\"];\n")
	for _, node := range g.Nodes {
		label := node.ID.Kind + "\n" + node.ID.Name
		if node.ID.Namespace != "" {
			label = node.ID.Kind + "\n" + node.ID.Namespace + "/" + node.ID.Name
		}
		attrs := []string{"label=" + quote(label)}
		switch {
		case node.Missing:
			attrs = append(attrs, `style=dashed`, `color=red`, `xlabel="missing"`)
		case node.Unreadable:
			attrs = append(attrs, `style=dotted`, `color=grey`, `xlabel="unreadable"`)
		}
		_, _ = fmt.Fprintf(&b, "  %s [%s];\n", quote(node.ID.String()), strings.Join(attrs, ", "))
	}
	for _, edge := range g.Edges {
		attrs := []string{"label=" + quote(string(edge.Type))}
		if edge.Optional {
			attrs = append(attrs, "style=dashed")
		}
		_, _ = fmt.Fprintf(&b, "  %s -> %s [%s];\n", quote(edge.From.String()), quote(edge.To.String()), strings.Join(attrs, ", "))
	}
	b.WriteString("}\n")
	return b.String()
}

func quote(value string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`).Replace(value) + `"`
}

type builder struct {
	nodes   map[string]*Node
	objects map[string]unstructured.Unstructured
	edges   map[string]*Edge
}

// infer adds every edge that leaves id.
func (b *builder) infer(id resource.ID, obj unstructured.Unstructured, keys []string) {
	for _, owner := range obj.GetOwnerReferences() {
		gv, err := schema.ParseGroupVersion(owner.APIVersion)
		if err != nil || owner.Kind == "" || owner.Name == "" {
			continue
		}
		ownerID := resource.ID{Group: gv.Group, Version: gv.Version, Kind: owner.Kind, Namespace: id.Namespace, Name: owner.Name}
		// An owner is not a dependency: only one that is present is linked.
		if node, ok := b.nodes[Key(ownerID)]; ok {
			b.link(node.ID, id, EdgeOwns, false)
		}
	}
	switch {
	case id.Group == "" && id.Kind == "Service", id.Group == "policy" && id.Kind == "PodDisruptionBudget":
		if s, ok := Selector(obj); ok {
			b.selects(id, s, keys, id.Kind == "Service")
		}
	case id.Group == "networking.k8s.io" && id.Kind == "Ingress":
		b.ingress(id, obj)
	case id.Group == "autoscaling" && id.Kind == "HorizontalPodAutoscaler":
		b.autoscaler(id, obj)
	case id.Group == "" && id.Kind == "PersistentVolumeClaim":
		if volume, _, _ := unstructured.NestedString(obj.Object, "spec", "volumeName"); volume != "" {
			b.reference(id, resource.ID{Version: "v1", Kind: "PersistentVolume", Name: volume}, EdgeBinds, false)
		}
	case id.Group == "discovery.k8s.io" && id.Kind == "EndpointSlice":
		service := obj.GetLabels()["kubernetes.io/service-name"]
		if node, ok := b.nodes[Key(resource.ID{Version: "v1", Kind: "Service", Namespace: id.Namespace, Name: service})]; ok && service != "" {
			b.link(node.ID, id, EdgeEndpoints, false)
		}
	}
	if spec, ok := PodSpec(obj); ok {
		b.podSpec(id, spec)
	}
}

// Selector returns the label selector in obj's spec.selector: a plain label
// map for a Service, which selects nothing when empty, and a LabelSelector
// for every other kind.
func Selector(obj unstructured.Unstructured) (labels.Selector, bool) {
	if obj.GroupVersionKind().GroupKind() == (schema.GroupKind{Kind: "Service"}) {
		selector, _, err := unstructured.NestedStringMap(obj.Object, "spec", "selector")
		if err != nil || len(selector) == 0 {
			return nil, false
		}
		return labels.SelectorFromSet(selector), true
	}
	raw, found, err := unstructured.NestedMap(obj.Object, "spec", "selector")
	if err != nil || !found {
		return nil, false
	}
	var selector metav1.LabelSelector
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(raw, &selector); err != nil {
		return nil, false
	}
	parsed, err := metav1.LabelSelectorAsSelector(&selector)
	if err != nil {
		return nil, false
	}
	return parsed, true
}

// selects links id to the Pods in its namespace that selector matches and,
// when workloads is set, to the workloads whose Pod template it matches.
func (b *builder) selects(id resource.ID, selector labels.Selector, keys []string, workloads bool) {
	for _, key := range keys {
		target := b.nodes[key].ID
		if target.Namespace != id.Namespace || target == id {
			continue
		}
		obj := b.objects[key]
		switch {
		case target.Group == "" && target.Kind == "Pod":
			if selector.Matches(labels.Set(obj.GetLabels())) {
				b.link(id, target, EdgeSelects, false)
			}
		case workloads && templated(target):
			template, _, _ := unstructured.NestedStringMap(obj.Object, "spec", "template", "metadata", "labels")
			if len(template) > 0 && selector.Matches(labels.Set(template)) {
				b.link(id, target, EdgeSelects, false)
			}
		}
	}
}

func (b *builder) ingress(id resource.ID, obj unstructured.Unstructured) {
	backends := []any{}
	if backend, found, _ := unstructured.NestedMap(obj.Object, "spec", "defaultBackend"); found {
		backends = append(backends, backend)
	}
	rules, _, _ := unstructured.NestedSlice(obj.Object, "spec", "rules")
	for _, rule := range rules {
		paths, _, _ := unstructured.NestedSlice(asMap(rule), "http", "paths")
		for _, path := range paths {
			if backend, ok := asMap(path)["backend"]; ok {
				backends = append(backends, backend)
			}
		}
	}
	for _, backend := range backends {
		name, _, _ := unstructured.NestedString(asMap(backend), "service", "name")
		if name != "" {
			b.reference(id, resource.ID{Version: "v1", Kind: "Service", Namespace: id.Namespace, Name: name}, EdgeRoutes, false)
		}
	}
}

func (b *builder) autoscaler(id resource.ID, obj unstructured.Unstructured) {
	target, _, _ := unstructured.NestedStringMap(obj.Object, "spec", "scaleTargetRef")
	gv, err := schema.ParseGroupVersion(target["apiVersion"])
	if err != nil || target["kind"] == "" || target["name"] == "" {
		return
	}
	b.reference(id, resource.ID{Group: gv.Group, Version: gv.Version, Kind: target["kind"], Namespace: id.Namespace, Name: target["name"]}, EdgeScales, false)
}

// podSpec links a Pod or workload to the claims, ConfigMaps, Secrets and
// ServiceAccount its Pod spec needs.
func (b *builder) podSpec(id resource.ID, spec map[string]any) {
	local := func(kind, name string) resource.ID {
		return resource.ID{Version: "v1", Kind: kind, Namespace: id.Namespace, Name: name}
	}
	for _, ref := range PodReferences(spec) {
		switch ref.Kind {
		case "PersistentVolumeClaim":
			b.reference(id, local(ref.Kind, ref.Name), EdgeMounts, ref.Optional)
		case "ServiceAccount":
			b.reference(id, local(ref.Kind, ref.Name), EdgeRunsAs, ref.Optional)
		default:
			b.reference(id, local(ref.Kind, ref.Name), EdgeUses, ref.Optional)
		}
	}
}

// reference links from to the object to refers to, adding it as a missing
// node when the input does not hold it.
func (b *builder) reference(from, to resource.ID, kind EdgeType, optional bool) {
	node, ok := b.nodes[Key(to)]
	if !ok {
		node = &Node{ID: to, Missing: true}
		b.nodes[Key(to)] = node
	}
	b.link(from, node.ID, kind, optional)
}

func (b *builder) link(from, to resource.ID, kind EdgeType, optional bool) {
	key := Key(from) + "->" + Key(to) + ":" + string(kind)
	if edge, ok := b.edges[key]; ok {
		// One required reference makes the edge required.
		edge.Optional = edge.Optional && optional
		return
	}
	b.edges[key] = &Edge{From: from, To: to, Type: kind, Optional: optional}
}

func (b *builder) graph() Graph {
	g := Graph{Nodes: make([]Node, 0, len(b.nodes)), Edges: make([]Edge, 0, len(b.edges)), index: map[string]int{}}
	for _, node := range b.nodes {
		g.Nodes = append(g.Nodes, *node)
	}
	sort.Slice(g.Nodes, func(i, j int) bool { return g.Nodes[i].ID.String() < g.Nodes[j].ID.String() })
	for i, node := range g.Nodes {
		g.index[Key(node.ID)] = i
	}
	for _, edge := range b.edges {
		g.Edges = append(g.Edges, *edge)
	}
	sort.Slice(g.Edges, func(i, j int) bool { return edgeKey(g.Edges[i]) < edgeKey(g.Edges[j]) })
	g.out = map[string][]Edge{}
	for _, edge := range g.Edges {
		g.out[Key(edge.From)] = append(g.out[Key(edge.From)], edge)
	}
	return g
}

// Reference is an object a Pod spec needs.
type Reference struct {
	Kind     string
	Name     string
	Optional bool
	// PullOnly is true when the reference is only an imagePullSecret, which
	// matters only when pulling an image fails.
	PullOnly bool
}

// PodSpec returns the Pod spec of a Pod or of a workload's Pod template.
func PodSpec(obj unstructured.Unstructured) (map[string]any, bool) {
	gv, err := schema.ParseGroupVersion(obj.GetAPIVersion())
	if err != nil {
		return nil, false
	}
	id := resource.ID{Group: gv.Group, Kind: obj.GetKind()}
	var fields []string
	switch {
	case id.Group == "" && id.Kind == "Pod":
		fields = []string{"spec"}
	case id.Group == "batch" && id.Kind == "CronJob":
		fields = []string{"spec", "jobTemplate", "spec", "template", "spec"}
	case templated(id):
		fields = []string{"spec", "template", "spec"}
	default:
		return nil, false
	}
	spec, found, _ := unstructured.NestedMap(obj.Object, fields...)
	return spec, found
}

// PodReferences lists, sorted and without duplicates, the claims, ConfigMaps,
// Secrets and ServiceAccount a Pod spec refers to. A reference is optional
// only when every mention of it is.
func PodReferences(spec map[string]any) []Reference {
	refs := map[string]*Reference{}
	add := func(kind, name string, optional, pull bool) {
		if name == "" {
			return
		}
		key := kind + "/" + name
		if ref, ok := refs[key]; ok {
			ref.Optional = ref.Optional && optional
			ref.PullOnly = ref.PullOnly && pull
			return
		}
		refs[key] = &Reference{Kind: kind, Name: name, Optional: optional, PullOnly: pull}
	}
	containers := append(anySlice(spec["initContainers"]), anySlice(spec["containers"])...)
	containers = append(containers, anySlice(spec["ephemeralContainers"])...)
	for _, raw := range containers {
		container := asMap(raw)
		for _, source := range anySlice(container["envFrom"]) {
			for kind, field := range map[string]string{"ConfigMap": "configMapRef", "Secret": "secretRef"} {
				if ref, ok := asMap(source)[field].(map[string]any); ok {
					add(kind, stringField(ref, "name"), boolField(ref, "optional"), false)
				}
			}
		}
		for _, env := range anySlice(container["env"]) {
			from := asMap(asMap(env)["valueFrom"])
			for kind, field := range map[string]string{"ConfigMap": "configMapKeyRef", "Secret": "secretKeyRef"} {
				if ref, ok := from[field].(map[string]any); ok {
					add(kind, stringField(ref, "name"), boolField(ref, "optional"), false)
				}
			}
		}
	}
	for _, raw := range anySlice(spec["volumes"]) {
		volume := asMap(raw)
		if ref, ok := volume["configMap"].(map[string]any); ok {
			add("ConfigMap", stringField(ref, "name"), boolField(ref, "optional"), false)
		}
		if ref, ok := volume["secret"].(map[string]any); ok {
			add("Secret", stringField(ref, "secretName"), boolField(ref, "optional"), false)
		}
		if ref, ok := volume["persistentVolumeClaim"].(map[string]any); ok {
			add("PersistentVolumeClaim", stringField(ref, "claimName"), false, false)
		}
		for _, projected := range anySlice(asMap(volume["projected"])["sources"]) {
			source := asMap(projected)
			if ref, ok := source["configMap"].(map[string]any); ok {
				add("ConfigMap", stringField(ref, "name"), boolField(ref, "optional"), false)
			}
			if ref, ok := source["secret"].(map[string]any); ok {
				add("Secret", stringField(ref, "name"), boolField(ref, "optional"), false)
			}
		}
	}
	for _, raw := range anySlice(spec["imagePullSecrets"]) {
		add("Secret", stringField(asMap(raw), "name"), false, true)
	}
	account := stringField(spec, "serviceAccountName")
	if account == "" {
		account = stringField(spec, "serviceAccount")
	}
	add("ServiceAccount", account, false, false)
	out := make([]Reference, 0, len(refs))
	for _, ref := range refs {
		out = append(out, *ref)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// templated reports whether id is a built-in workload with a Pod template.
func templated(id resource.ID) bool {
	switch {
	case id.Group == "apps" && (id.Kind == "Deployment" || id.Kind == "StatefulSet" || id.Kind == "DaemonSet" || id.Kind == "ReplicaSet"):
		return true
	case id.Group == "batch" && id.Kind == "Job":
		return true
	case id.Group == "" && id.Kind == "ReplicationController":
		return true
	}
	return false
}

// Key identifies an object regardless of the API version it was read at.
func Key(id resource.ID) string {
	return id.Group + "/" + id.Kind + "/" + id.Namespace + "/" + id.Name
}

func edgeKey(edge Edge) string {
	return edge.From.String() + "->" + edge.To.String() + ":" + string(edge.Type)
}

func asMap(v any) map[string]any {
	out, _ := v.(map[string]any)
	return out
}

func anySlice(v any) []any {
	out, _ := v.([]any)
	return out
}

func stringField(m map[string]any, key string) string {
	out, _ := m[key].(string)
	return out
}

func boolField(m map[string]any, key string) bool {
	out, _ := m[key].(bool)
	return out
}
