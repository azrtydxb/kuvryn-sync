package graph

import (
	"fmt"
	"sort"

	"github.com/azrtydxb/solder/internal/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// EdgeType names an inferred Kubernetes relationship.
type EdgeType string

const (
	EdgeOwns      EdgeType = "Owns"
	EdgeSelects   EdgeType = "Selects"
	EdgeMountsPVC EdgeType = "MountsPVC"
	EdgeDependsOn EdgeType = "DependsOn"
)

// Edge is a deterministic directed relationship between resources.
type Edge struct {
	From resource.ID `json:"from"`
	To   resource.ID `json:"to"`
	Type EdgeType    `json:"type"`
}

// Graph contains nodes and inferred edges.
type Graph struct {
	Nodes []resource.ID `json:"nodes"`
	Edges []Edge        `json:"edges"`
}

// Build infers common built-in relationships across rendered/live objects.
func Build(objects []unstructured.Unstructured) (Graph, error) {
	nodes := []resource.ID{}
	byKey := map[string]unstructured.Unstructured{}
	for _, obj := range objects {
		id, err := resource.FromObject(obj)
		if err != nil {
			return Graph{}, err
		}
		nodes = append(nodes, id)
		byKey[id.String()] = obj
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].String() < nodes[j].String() })
	edges := []Edge{}
	for _, from := range nodes {
		obj := byKey[from.String()]
		for _, owner := range obj.GetOwnerReferences() {
			for _, to := range nodes {
				if to.Kind == owner.Kind && to.Name == owner.Name && to.Namespace == from.Namespace {
					edges = append(edges, Edge{From: to, To: from, Type: EdgeOwns})
				}
			}
		}
		if from.Kind == "Service" {
			selector := stringMap(obj.Object["spec"], "selector")
			for _, to := range nodes {
				candidate := byKey[to.String()]
				if sameNamespace(from, to) && workload(to.Kind) && labelsMatch(selector, candidate.GetLabels()) {
					edges = append(edges, Edge{From: from, To: to, Type: EdgeSelects})
				}
			}
		}
		for _, claim := range pvcClaims(obj) {
			for _, to := range nodes {
				if sameNamespace(from, to) && to.Kind == "PersistentVolumeClaim" && to.Name == claim {
					edges = append(edges, Edge{From: from, To: to, Type: EdgeMountsPVC})
				}
			}
		}
		for _, dep := range configDeps(obj) {
			for _, to := range nodes {
				if sameNamespace(from, to) && to.Name == dep && (to.Kind == "ConfigMap" || to.Kind == "Secret") {
					edges = append(edges, Edge{From: from, To: to, Type: EdgeDependsOn})
				}
			}
		}
	}
	sort.Slice(edges, func(i, j int) bool { return edgeKey(edges[i]) < edgeKey(edges[j]) })
	return Graph{Nodes: nodes, Edges: edges}, nil
}

// Children returns resources directly reached from id.
func (g Graph) Children(id resource.ID) []resource.ID {
	out := []resource.ID{}
	for _, edge := range g.Edges {
		if edge.From == id {
			out = append(out, edge.To)
		}
	}
	return out
}

func sameNamespace(a, b resource.ID) bool { return a.Namespace == b.Namespace }

func workload(kind string) bool {
	switch kind {
	case "Deployment", "StatefulSet", "DaemonSet", "ReplicaSet", "Pod", "Job", "CronJob":
		return true
	default:
		return false
	}
}

func labelsMatch(selector, labels map[string]string) bool {
	if len(selector) == 0 {
		return false
	}
	for k, v := range selector {
		if labels[k] != v {
			return false
		}
	}
	return true
}

func stringMap(root any, key string) map[string]string {
	m, ok := root.(map[string]any)
	if !ok {
		return nil
	}
	child, ok := m[key].(map[string]any)
	if !ok {
		return nil
	}
	out := map[string]string{}
	for k, v := range child {
		out[k] = fmt.Sprint(v)
	}
	return out
}

func pvcClaims(obj unstructured.Unstructured) []string {
	volumes, _, _ := unstructured.NestedSlice(obj.Object, "spec", "template", "spec", "volumes")
	if len(volumes) == 0 {
		volumes, _, _ = unstructured.NestedSlice(obj.Object, "spec", "volumes")
	}
	claims := []string{}
	for _, volume := range volumes {
		m, ok := volume.(map[string]any)
		if !ok {
			continue
		}
		pvc, ok := m["persistentVolumeClaim"].(map[string]any)
		if !ok {
			continue
		}
		if name, ok := pvc["claimName"].(string); ok {
			claims = append(claims, name)
		}
	}
	return claims
}

func configDeps(obj unstructured.Unstructured) []string {
	containers, _, _ := unstructured.NestedSlice(obj.Object, "spec", "template", "spec", "containers")
	deps := []string{}
	for _, container := range containers {
		m, ok := container.(map[string]any)
		if !ok {
			continue
		}
		for _, envFrom := range anySlice(m["envFrom"]) {
			em, ok := envFrom.(map[string]any)
			if !ok {
				continue
			}
			for _, key := range []string{"configMapRef", "secretRef"} {
				ref, ok := em[key].(map[string]any)
				if ok {
					if name, ok := ref["name"].(string); ok {
						deps = append(deps, name)
					}
				}
			}
		}
	}
	return deps
}

func anySlice(v any) []any {
	out, _ := v.([]any)
	return out
}

func edgeKey(edge Edge) string {
	return edge.From.String() + "->" + edge.To.String() + ":" + string(edge.Type)
}
