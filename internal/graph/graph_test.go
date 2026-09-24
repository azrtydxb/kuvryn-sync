package graph

import (
	"encoding/json"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"

	"github.com/azrtydxb/solder/internal/resource"
)

const ns = "payments"

func object(apiVersion, kind, name string, fields map[string]any) unstructured.Unstructured {
	obj := unstructured.Unstructured{Object: map[string]any{"apiVersion": apiVersion, "kind": kind}}
	for key, value := range fields {
		obj.Object[key] = value
	}
	obj.SetName(name)
	if kind != "PersistentVolume" {
		obj.SetNamespace(ns)
	}
	return obj
}

func owned(obj unstructured.Unstructured, apiVersion, kind, name string) unstructured.Unstructured {
	obj.SetOwnerReferences([]metav1.OwnerReference{{APIVersion: apiVersion, Kind: kind, Name: name, UID: types.UID(name)}})
	return obj
}

func labelled(obj unstructured.Unstructured, labels map[string]string) unstructured.Unstructured {
	obj.SetLabels(labels)
	return obj
}

func podTemplate(spec map[string]any) map[string]any {
	return map[string]any{
		"selector": map[string]any{"matchLabels": map[string]any{"app": "api"}},
		"template": map[string]any{"metadata": map[string]any{"labels": map[string]any{"app": "api"}}, "spec": spec},
	}
}

func id(apiVersion, kind, name string) resource.ID {
	obj := object(apiVersion, kind, name, nil)
	out, err := resource.FromObject(obj)
	if err != nil {
		panic(err)
	}
	return out
}

func hasEdge(g Graph, from, to resource.ID, kind EdgeType) bool {
	for _, edge := range g.Edges {
		if edge.From == from && edge.To == to && edge.Type == kind {
			return true
		}
	}
	return false
}

func requireEdges(t *testing.T, g Graph, want ...Edge) {
	t.Helper()
	for _, edge := range want {
		if !hasEdge(g, edge.From, edge.To, edge.Type) {
			t.Errorf("missing edge %s -%s-> %s; edges: %v", edge.From, edge.Type, edge.To, g.Edges)
		}
	}
}

func TestOwnerReferencesLinkOwnersToOwnedObjects(t *testing.T) {
	deployment := id("apps/v1", "Deployment", "api")
	replicaSet := id("apps/v1", "ReplicaSet", "api-1")
	pod := id("v1", "Pod", "api-1-a")
	job := id("batch/v1", "Job", "migrate")
	jobPod := id("v1", "Pod", "migrate-x")
	widget := id("example.com/v1", "Widget", "w")
	child := id("v1", "ConfigMap", "w-state")
	g := Build([]unstructured.Unstructured{
		object("apps/v1", "Deployment", "api", nil),
		owned(object("apps/v1", "ReplicaSet", "api-1", nil), "apps/v1", "Deployment", "api"),
		owned(object("v1", "Pod", "api-1-a", nil), "apps/v1", "ReplicaSet", "api-1"),
		object("batch/v1", "Job", "migrate", nil),
		owned(object("v1", "Pod", "migrate-x", nil), "batch/v1", "Job", "migrate"),
		object("example.com/v1", "Widget", "w", nil),
		// An owner read at another API version is still the same object.
		owned(object("v1", "ConfigMap", "w-state", nil), "example.com/v2", "Widget", "w"),
		// An owner that is not in the input is not a dependency.
		owned(object("v1", "ConfigMap", "orphan", nil), "example.com/v1", "Gadget", "gone"),
	})
	requireEdges(t, g,
		Edge{From: deployment, To: replicaSet, Type: EdgeOwns},
		Edge{From: replicaSet, To: pod, Type: EdgeOwns},
		Edge{From: job, To: jobPod, Type: EdgeOwns},
		Edge{From: widget, To: child, Type: EdgeOwns},
	)
	if _, ok := g.Node(id("example.com/v1", "Gadget", "gone")); ok {
		t.Fatalf("absent owner became a node: %v", g.Nodes)
	}
}

func TestServiceLinksEndpointSlicesPodsAndWorkloads(t *testing.T) {
	service := id("v1", "Service", "api")
	g := Build([]unstructured.Unstructured{
		object("v1", "Service", "api", map[string]any{"spec": map[string]any{"selector": map[string]any{"app": "api"}}}),
		object("v1", "Service", "headless", map[string]any{"spec": map[string]any{}}),
		labelled(object("discovery.k8s.io/v1", "EndpointSlice", "api-abc", nil), map[string]string{"kubernetes.io/service-name": "api"}),
		labelled(object("v1", "Pod", "api-1", nil), map[string]string{"app": "api", "pod-template-hash": "1"}),
		labelled(object("v1", "Pod", "other", nil), map[string]string{"app": "web"}),
		object("apps/v1", "Deployment", "api", map[string]any{"spec": podTemplate(map[string]any{})}),
		object("apps/v1", "StatefulSet", "db", map[string]any{"spec": map[string]any{"template": map[string]any{"metadata": map[string]any{"labels": map[string]any{"app": "db"}}}}}),
	})
	requireEdges(t, g,
		Edge{From: service, To: id("discovery.k8s.io/v1", "EndpointSlice", "api-abc"), Type: EdgeEndpoints},
		Edge{From: service, To: id("v1", "Pod", "api-1"), Type: EdgeSelects},
		Edge{From: service, To: id("apps/v1", "Deployment", "api"), Type: EdgeSelects},
	)
	if hasEdge(g, service, id("v1", "Pod", "other"), EdgeSelects) || hasEdge(g, service, id("apps/v1", "StatefulSet", "db"), EdgeSelects) {
		t.Fatalf("service selected objects its selector does not match: %v", g.Edges)
	}
	if len(g.Out(id("v1", "Service", "headless"))) != 0 {
		t.Fatalf("a Service without selector selected something: %v", g.Out(id("v1", "Service", "headless")))
	}
}

func TestIngressRoutesToRuleAndDefaultBackends(t *testing.T) {
	ingress := id("networking.k8s.io/v1", "Ingress", "web")
	g := Build([]unstructured.Unstructured{
		object("networking.k8s.io/v1", "Ingress", "web", map[string]any{"spec": map[string]any{
			"defaultBackend": map[string]any{"service": map[string]any{"name": "fallback"}},
			"rules": []any{map[string]any{"http": map[string]any{"paths": []any{
				map[string]any{"path": "/", "backend": map[string]any{"service": map[string]any{"name": "api", "port": map[string]any{"number": int64(80)}}}},
			}}}},
		}}),
		object("v1", "Service", "api", nil),
	})
	requireEdges(t, g,
		Edge{From: ingress, To: id("v1", "Service", "api"), Type: EdgeRoutes},
		Edge{From: ingress, To: id("v1", "Service", "fallback"), Type: EdgeRoutes},
	)
	if node, _ := g.Node(id("v1", "Service", "fallback")); !node.Missing {
		t.Fatalf("absent default backend is not missing: %#v", node)
	}
}

func TestAutoscalerAndDisruptionBudgetTargets(t *testing.T) {
	g := Build([]unstructured.Unstructured{
		object("autoscaling/v2", "HorizontalPodAutoscaler", "api", map[string]any{"spec": map[string]any{
			"scaleTargetRef": map[string]any{"apiVersion": "apps/v1", "kind": "Deployment", "name": "api"},
		}}),
		object("apps/v1", "Deployment", "api", nil),
		object("policy/v1", "PodDisruptionBudget", "api", map[string]any{"spec": map[string]any{
			"selector": map[string]any{"matchExpressions": []any{map[string]any{"key": "app", "operator": "In", "values": []any{"api"}}}},
		}}),
		labelled(object("v1", "Pod", "api-1", nil), map[string]string{"app": "api"}),
		labelled(object("v1", "Pod", "web-1", nil), map[string]string{"app": "web"}),
	})
	budget := id("policy/v1", "PodDisruptionBudget", "api")
	requireEdges(t, g,
		Edge{From: id("autoscaling/v2", "HorizontalPodAutoscaler", "api"), To: id("apps/v1", "Deployment", "api"), Type: EdgeScales},
		Edge{From: budget, To: id("v1", "Pod", "api-1"), Type: EdgeSelects},
	)
	if hasEdge(g, budget, id("v1", "Pod", "web-1"), EdgeSelects) {
		t.Fatal("budget selected a Pod its selector does not match")
	}
}

func TestStorageEdges(t *testing.T) {
	g := Build([]unstructured.Unstructured{
		object("v1", "PersistentVolumeClaim", "data", map[string]any{"spec": map[string]any{"volumeName": "pv-1"}}),
		object("apps/v1", "StatefulSet", "db", map[string]any{"spec": podTemplate(map[string]any{
			"volumes": []any{map[string]any{"name": "data", "persistentVolumeClaim": map[string]any{"claimName": "data"}}},
		})}),
		object("v1", "Pod", "db-0", map[string]any{"spec": map[string]any{
			"volumes": []any{map[string]any{"name": "scratch", "persistentVolumeClaim": map[string]any{"claimName": "scratch"}}},
		}}),
	})
	pv := resource.ID{Version: "v1", Kind: "PersistentVolume", Name: "pv-1"}
	requireEdges(t, g,
		Edge{From: id("v1", "PersistentVolumeClaim", "data"), To: pv, Type: EdgeBinds},
		Edge{From: id("apps/v1", "StatefulSet", "db"), To: id("v1", "PersistentVolumeClaim", "data"), Type: EdgeMounts},
		Edge{From: id("v1", "Pod", "db-0"), To: id("v1", "PersistentVolumeClaim", "scratch"), Type: EdgeMounts},
	)
	if node, _ := g.Node(pv); !node.Missing || node.ID.Namespace != "" {
		t.Fatalf("absent cluster-scoped volume = %#v", node)
	}
}

func TestPodSpecReferencesConfigurationAndIdentity(t *testing.T) {
	spec := map[string]any{
		"serviceAccountName": "api-runner",
		"imagePullSecrets":   []any{map[string]any{"name": "registry"}},
		"initContainers": []any{map[string]any{
			"name":    "init",
			"envFrom": []any{map[string]any{"secretRef": map[string]any{"name": "init-env"}}},
		}},
		"containers": []any{map[string]any{
			"name": "api",
			"envFrom": []any{
				map[string]any{"configMapRef": map[string]any{"name": "api-env"}},
				map[string]any{"secretRef": map[string]any{"name": "api-secret-env", "optional": true}},
			},
			"env": []any{
				map[string]any{"name": "A", "valueFrom": map[string]any{"configMapKeyRef": map[string]any{"name": "flags", "key": "a"}}},
				map[string]any{"name": "B", "valueFrom": map[string]any{"secretKeyRef": map[string]any{"name": "db", "key": "password"}}},
				map[string]any{"name": "C", "value": "plain"},
			},
		}},
		"volumes": []any{
			map[string]any{"name": "cfg", "configMap": map[string]any{"name": "files"}},
			map[string]any{"name": "tls", "secret": map[string]any{"secretName": "tls"}},
			map[string]any{"name": "bundle", "projected": map[string]any{"sources": []any{
				map[string]any{"configMap": map[string]any{"name": "ca"}},
				map[string]any{"secret": map[string]any{"name": "token"}},
				map[string]any{"serviceAccountToken": map[string]any{"path": "t"}},
			}}},
		},
	}
	for _, workload := range []unstructured.Unstructured{
		object("v1", "Pod", "api", map[string]any{"spec": spec}),
		object("apps/v1", "Deployment", "api", map[string]any{"spec": podTemplate(spec)}),
		object("batch/v1", "CronJob", "api", map[string]any{"spec": map[string]any{"jobTemplate": map[string]any{"spec": podTemplate(spec)}}}),
	} {
		t.Run(workload.GetKind(), func(t *testing.T) {
			g := Build([]unstructured.Unstructured{workload})
			from, _ := resource.FromObject(workload)
			requireEdges(t, g,
				Edge{From: from, To: id("v1", "ServiceAccount", "api-runner"), Type: EdgeRunsAs},
				Edge{From: from, To: id("v1", "Secret", "registry"), Type: EdgeUses},
				Edge{From: from, To: id("v1", "Secret", "init-env"), Type: EdgeUses},
				Edge{From: from, To: id("v1", "ConfigMap", "api-env"), Type: EdgeUses},
				Edge{From: from, To: id("v1", "Secret", "api-secret-env"), Type: EdgeUses},
				Edge{From: from, To: id("v1", "ConfigMap", "flags"), Type: EdgeUses},
				Edge{From: from, To: id("v1", "Secret", "db"), Type: EdgeUses},
				Edge{From: from, To: id("v1", "ConfigMap", "files"), Type: EdgeUses},
				Edge{From: from, To: id("v1", "Secret", "tls"), Type: EdgeUses},
				Edge{From: from, To: id("v1", "ConfigMap", "ca"), Type: EdgeUses},
				Edge{From: from, To: id("v1", "Secret", "token"), Type: EdgeUses},
			)
			if got := len(g.Out(from)); got != 11 {
				t.Fatalf("got %d edges from %s, want 11: %v", got, from, g.Out(from))
			}
			for _, edge := range g.Out(from) {
				if wantOptional := edge.To.Name == "api-secret-env"; edge.Optional != wantOptional {
					t.Errorf("edge to %s optional=%v, want %v", edge.To, edge.Optional, wantOptional)
				}
			}
			if len(g.Missing()) != 11 {
				t.Fatalf("every absent reference must be a missing node, got %v", g.Missing())
			}
		})
	}
}

func TestPodReferencesMarksPullOnlySecrets(t *testing.T) {
	refs := PodReferences(map[string]any{
		"imagePullSecrets": []any{map[string]any{"name": "registry"}, map[string]any{"name": "shared"}},
		"containers": []any{map[string]any{"name": "a", "envFrom": []any{
			map[string]any{"secretRef": map[string]any{"name": "shared"}},
		}}},
	})
	pull := map[string]bool{}
	for _, ref := range refs {
		pull[ref.Name] = ref.PullOnly
	}
	if !pull["registry"] || pull["shared"] {
		t.Fatalf("pull-only marking = %v", pull)
	}
}

func TestMissingNodesAndPresentReferences(t *testing.T) {
	g := Build([]unstructured.Unstructured{
		object("v1", "Pod", "api", map[string]any{"spec": map[string]any{"containers": []any{map[string]any{
			"name": "api", "envFrom": []any{
				map[string]any{"secretRef": map[string]any{"name": "present"}},
				map[string]any{"secretRef": map[string]any{"name": "absent"}},
			},
		}}}}),
		object("v1", "Secret", "present", nil),
	})
	present, _ := g.Node(id("v1", "Secret", "present"))
	absent, _ := g.Node(id("v1", "Secret", "absent"))
	if present.Missing || !absent.Missing {
		t.Fatalf("present=%#v absent=%#v", present, absent)
	}
	g.MarkUnreadable(id("v1", "Secret", "absent"), id("v1", "Secret", "present"))
	present, _ = g.Node(id("v1", "Secret", "present"))
	absent, _ = g.Node(id("v1", "Secret", "absent"))
	if present.Unreadable || absent.Missing || !absent.Unreadable {
		t.Fatalf("after MarkUnreadable present=%#v absent=%#v", present, absent)
	}
}

func TestUnknownKindsAndInvalidObjectsNeverFail(t *testing.T) {
	g := Build([]unstructured.Unstructured{
		object("example.com/v1", "Widget", "w", map[string]any{"spec": map[string]any{"selector": "not-a-map", "template": 3}}),
		{Object: map[string]any{"apiVersion": "a/b/c", "kind": "Broken", "metadata": map[string]any{"name": "x"}}},
		{Object: map[string]any{"apiVersion": "v1", "kind": "ConfigMap"}},
		object("v1", "Service", "odd", map[string]any{"spec": "not-a-map"}),
		object("policy/v1", "PodDisruptionBudget", "odd", map[string]any{"spec": map[string]any{"selector": map[string]any{"matchExpressions": "bad"}}}),
	})
	if len(g.Nodes) != 3 || len(g.Edges) != 0 {
		t.Fatalf("nodes=%v edges=%v", g.Nodes, g.Edges)
	}
}

func TestJSONAndDOTAreStable(t *testing.T) {
	objects := []unstructured.Unstructured{
		object("apps/v1", "Deployment", "api", map[string]any{"spec": podTemplate(map[string]any{
			"containers": []any{map[string]any{"name": "api", "envFrom": []any{map[string]any{"secretRef": map[string]any{"name": "db"}}}}},
		})}),
		owned(object("apps/v1", "ReplicaSet", "api-1", nil), "apps/v1", "Deployment", "api"),
		object("v1", "Service", "api", map[string]any{"spec": map[string]any{"selector": map[string]any{"app": "api"}}}),
	}
	reversed := []unstructured.Unstructured{objects[2], objects[1], objects[0]}
	first, err := json.Marshal(Build(objects))
	if err != nil {
		t.Fatal(err)
	}
	second, err := json.Marshal(Build(reversed))
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatalf("JSON depends on input order:\n%s\n%s", first, second)
	}
	want := `{"nodes":[` +
		`{"id":"apps/v1/Deployment/payments/api","apiVersion":"apps/v1","kind":"Deployment","namespace":"payments","name":"api"},` +
		`{"id":"apps/v1/ReplicaSet/payments/api-1","apiVersion":"apps/v1","kind":"ReplicaSet","namespace":"payments","name":"api-1"},` +
		`{"id":"v1/Secret/payments/db","apiVersion":"v1","kind":"Secret","namespace":"payments","name":"db","missing":true},` +
		`{"id":"v1/Service/payments/api","apiVersion":"v1","kind":"Service","namespace":"payments","name":"api"}],` +
		`"edges":[` +
		`{"from":"apps/v1/Deployment/payments/api","to":"apps/v1/ReplicaSet/payments/api-1","type":"Owns"},` +
		`{"from":"apps/v1/Deployment/payments/api","to":"v1/Secret/payments/db","type":"Uses"},` +
		`{"from":"v1/Service/payments/api","to":"apps/v1/Deployment/payments/api","type":"Selects"}]}`
	if string(first) != want {
		t.Fatalf("JSON =\n%s\nwant\n%s", first, want)
	}
	dot := Build(objects).DOT()
	if dot != Build(reversed).DOT() {
		t.Fatal("DOT depends on input order")
	}
	for _, line := range []string{
		"digraph solder {",
		`"apps/v1/Deployment/payments/api" [label="Deployment\npayments/api"];`,
		`"v1/Secret/payments/db" [label="Secret\npayments/db", style=dashed, color=red, xlabel="missing"];`,
		`"apps/v1/Deployment/payments/api" -> "apps/v1/ReplicaSet/payments/api-1" [label="Owns"];`,
	} {
		if !strings.Contains(dot, line) {
			t.Errorf("DOT missing %q:\n%s", line, dot)
		}
	}
}
