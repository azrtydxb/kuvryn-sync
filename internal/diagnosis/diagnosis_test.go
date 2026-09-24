package diagnosis

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"

	corev1alpha1 "github.com/azrtydxb/solder/api/v1alpha1"
	"github.com/azrtydxb/solder/internal/graph"
	"github.com/azrtydxb/solder/internal/health"
	"github.com/azrtydxb/solder/internal/resource"
)

const ns = "payments"

func object(apiVersion, kind, name string, fields map[string]any) unstructured.Unstructured {
	obj := unstructured.Unstructured{Object: map[string]any{"apiVersion": apiVersion, "kind": kind}}
	for k, v := range fields {
		obj.Object[k] = v
	}
	obj.SetName(name)
	obj.SetNamespace(ns)
	return obj
}

func ownedBy(obj unstructured.Unstructured, owner unstructured.Unstructured) unstructured.Unstructured {
	obj.SetOwnerReferences([]metav1.OwnerReference{{APIVersion: owner.GetAPIVersion(), Kind: owner.GetKind(), Name: owner.GetName(), UID: "uid"}})
	return obj
}

func idOf(obj unstructured.Unstructured) resource.ID {
	id, err := resource.FromObject(obj)
	if err != nil {
		panic(err)
	}
	return id
}

func deployment(name string, spec map[string]any) unstructured.Unstructured {
	return object("apps/v1", "Deployment", name, map[string]any{"spec": map[string]any{
		"template": map[string]any{"metadata": map[string]any{"labels": map[string]any{"app": name}}, "spec": spec},
	}})
}

func replicaSet(owner unstructured.Unstructured, spec map[string]any) unstructured.Unstructured {
	rs := object("apps/v1", "ReplicaSet", owner.GetName()+"-1", map[string]any{"spec": map[string]any{
		"template": map[string]any{"spec": spec},
	}})
	return ownedBy(rs, owner)
}

func pod(name string, owner unstructured.Unstructured, spec, status map[string]any) unstructured.Unstructured {
	p := object("v1", "Pod", name, map[string]any{"spec": spec, "status": status})
	p.SetLabels(map[string]string{"app": strings.Split(owner.GetName(), "-")[0]})
	return ownedBy(p, owner)
}

func envFromSecret(name string, optional bool) map[string]any {
	return map[string]any{"containers": []any{map[string]any{
		"name": "api", "image": "api:1",
		"envFrom": []any{map[string]any{"secretRef": map[string]any{"name": name, "optional": optional}}},
	}}}
}

func waiting(reason, message string, extra map[string]any) map[string]any {
	status := map[string]any{"name": "api", "ready": false, "state": map[string]any{"waiting": map[string]any{"reason": reason, "message": message}}}
	for k, v := range extra {
		status[k] = v
	}
	return map[string]any{"phase": "Pending", "containerStatuses": []any{status}}
}

func unhealthy(objs ...unstructured.Unstructured) []health.Result {
	out := make([]health.Result, 0, len(objs))
	for _, obj := range objs {
		out = append(out, health.Result{Resource: idOf(obj), State: corev1alpha1.HealthStateProgressing, Reason: "ReplicasUnavailable", Message: "0/1 replicas available"})
	}
	return out
}

func diagnose(results []health.Result, objects ...unstructured.Unstructured) []Cause {
	return Build(context.Background(), Input{Results: results, Graph: graph.Build(objects), Objects: objects})
}

func requireOne(t *testing.T, causes []Cause) Cause {
	t.Helper()
	if len(causes) != 1 {
		t.Fatalf("got %d causes, want 1: %+v", len(causes), causes)
	}
	return causes[0]
}

func requireChain(t *testing.T, cause Cause, want ...unstructured.Unstructured) {
	t.Helper()
	ids := make([]resource.ID, 0, len(want))
	for _, obj := range want {
		ids = append(ids, idOf(obj))
	}
	if !reflect.DeepEqual(cause.Chain, ids) {
		t.Fatalf("chain = %v, want %v", cause.Chain, ids)
	}
}

// workload returns a Deployment whose only Pod has the given spec and status.
func workload(spec, status map[string]any) (unstructured.Unstructured, unstructured.Unstructured, unstructured.Unstructured) {
	d := deployment("api", spec)
	rs := replicaSet(d, spec)
	return d, rs, pod("api-1-a", rs, spec, status)
}

func TestContainerWaitingReasonsAreLeafEvidence(t *testing.T) {
	for _, reason := range []string{"ImagePullBackOff", "ErrImagePull", "CreateContainerError", "InvalidImageName"} {
		t.Run(reason, func(t *testing.T) {
			spec := map[string]any{"containers": []any{map[string]any{"name": "api", "image": "registry.example/api:1"}}}
			d, rs, p := workload(spec, waiting(reason, "Back-off pulling image registry.example/api:1", nil))
			cause := requireOne(t, diagnose(unhealthy(d), d, rs, p))
			if cause.Reason != reason || cause.Resource != idOf(p) || !strings.Contains(cause.Message, "registry.example/api:1") {
				t.Fatalf("cause = %+v", cause)
			}
			requireChain(t, cause, d, rs, p)
		})
	}
}

// containerOutput is what a container wrote to its termination log, or its
// log output with FallbackToLogsOnError.
const containerOutput = "panic: connecting to the database as admin"

func TestCrashLoopIncludesLastTermination(t *testing.T) {
	d, rs, p := workload(map[string]any{}, waiting("CrashLoopBackOff", "back-off 5m0s restarting failed container", map[string]any{
		"lastState": map[string]any{"terminated": map[string]any{"reason": "Error", "exitCode": int64(137), "message": containerOutput}},
	}))
	cause := requireOne(t, diagnose(unhealthy(d), d, rs, p))
	if cause.Reason != "CrashLoopBackOff" || !strings.Contains(cause.Message, "Error (exit code 137)") {
		t.Fatalf("cause = %+v", cause)
	}
	if strings.Contains(cause.Message, containerOutput) {
		t.Fatalf("container output reached the message: %q", cause.Message)
	}
}

func TestMissingSecretIsTheRootCause(t *testing.T) {
	spec := envFromSecret("db", false)
	d, rs, p := workload(spec, waiting("CreateContainerConfigError", `secret "db" not found`, nil))
	cause := requireOne(t, diagnose(unhealthy(d), d, rs, p))
	secret := object("v1", "Secret", "db", nil)
	if cause.Reason != "MissingSecret" || cause.Resource != idOf(secret) {
		t.Fatalf("cause = %+v", cause)
	}
	requireChain(t, cause, d, rs, p, secret)
	if !strings.Contains(cause.Message, "Secret payments/db does not exist") || !strings.Contains(cause.Message, "CreateContainerConfigError") {
		t.Fatalf("message = %q", cause.Message)
	}
}

func TestMissingConfigMapIsTheRootCause(t *testing.T) {
	spec := map[string]any{
		"containers": []any{map[string]any{"name": "api"}},
		"volumes":    []any{map[string]any{"name": "cfg", "configMap": map[string]any{"name": "settings"}}},
	}
	d, rs, p := workload(spec, waiting("ContainerCreating", "", nil))
	cause := requireOne(t, diagnose(unhealthy(d), d, rs, p))
	if cause.Reason != "MissingConfigMap" || cause.Resource.Name != "settings" {
		t.Fatalf("cause = %+v", cause)
	}
}

func TestReferencesThatCannotBeBlamed(t *testing.T) {
	t.Run("optional", func(t *testing.T) {
		d, rs, p := workload(envFromSecret("db", true), waiting("CrashLoopBackOff", "", nil))
		if cause := requireOne(t, diagnose(unhealthy(d), d, rs, p)); cause.Reason != "CrashLoopBackOff" {
			t.Fatalf("cause = %+v", cause)
		}
	})
	t.Run("unreadable", func(t *testing.T) {
		d, rs, p := workload(envFromSecret("db", false), waiting("CreateContainerConfigError", `secret "db" not found`, nil))
		g := graph.Build([]unstructured.Unstructured{d, rs, p})
		g.MarkUnreadable(idOf(object("v1", "Secret", "db", nil)))
		cause := requireOne(t, Build(context.Background(), Input{Results: unhealthy(d), Graph: g, Objects: []unstructured.Unstructured{d, rs, p}}))
		if cause.Reason != "CreateContainerConfigError" || cause.Resource != idOf(p) {
			t.Fatalf("cause = %+v", cause)
		}
	})
	t.Run("pull secret while the image pulls", func(t *testing.T) {
		spec := map[string]any{"containers": []any{map[string]any{"name": "api"}}, "imagePullSecrets": []any{map[string]any{"name": "registry"}}}
		d, rs, p := workload(spec, waiting("CrashLoopBackOff", "", nil))
		if cause := requireOne(t, diagnose(unhealthy(d), d, rs, p)); cause.Reason != "CrashLoopBackOff" {
			t.Fatalf("cause = %+v", cause)
		}
	})
	t.Run("pull secret while pulling fails", func(t *testing.T) {
		spec := map[string]any{"containers": []any{map[string]any{"name": "api"}}, "imagePullSecrets": []any{map[string]any{"name": "registry"}}}
		d, rs, p := workload(spec, waiting("ImagePullBackOff", "", nil))
		if cause := requireOne(t, diagnose(unhealthy(d), d, rs, p)); cause.Reason != "MissingSecret" || cause.Resource.Name != "registry" {
			t.Fatalf("cause = %+v", cause)
		}
	})
	t.Run("running pod", func(t *testing.T) {
		running := map[string]any{"phase": "Running", "containerStatuses": []any{map[string]any{"name": "api", "ready": true}}}
		d, rs, p := workload(envFromSecret("db", false), running)
		// The Pod runs, so the ReplicaSet's template reference is next.
		cause := requireOne(t, diagnose(unhealthy(d), d, rs, p))
		requireChain(t, cause, d, rs, object("v1", "Secret", "db", nil))
	})
}

func TestUnschedulablePod(t *testing.T) {
	d, rs, p := workload(map[string]any{}, map[string]any{"phase": "Pending", "conditions": []any{map[string]any{
		"type": "PodScheduled", "status": "False", "reason": "Unschedulable", "message": "0/3 nodes are available: 3 Insufficient cpu.",
	}}})
	cause := requireOne(t, diagnose(unhealthy(d), d, rs, p))
	if cause.Reason != "Unschedulable" || !strings.Contains(cause.Message, "Insufficient cpu") {
		t.Fatalf("cause = %+v", cause)
	}
}

func TestPendingClaim(t *testing.T) {
	spec := map[string]any{"volumes": []any{map[string]any{"name": "data", "persistentVolumeClaim": map[string]any{"claimName": "data"}}}}
	sts := object("apps/v1", "StatefulSet", "db", map[string]any{"spec": map[string]any{"template": map[string]any{"spec": spec}}})
	p := pod("db-0", sts, spec, map[string]any{"phase": "Pending", "conditions": []any{map[string]any{
		"type": "PodScheduled", "status": "False", "reason": "Unschedulable", "message": "pod has unbound immediate PersistentVolumeClaims",
	}}})
	claim := object("v1", "PersistentVolumeClaim", "data", map[string]any{"spec": map[string]any{"storageClassName": "fast"}, "status": map[string]any{"phase": "Pending"}})
	cause := requireOne(t, diagnose(unhealthy(sts), sts, p, claim))
	if cause.Reason != "ClaimPending" || !strings.Contains(cause.Message, "fast") {
		t.Fatalf("cause = %+v", cause)
	}
	requireChain(t, cause, sts, p, claim)
}

func TestServiceWithoutReadyEndpoints(t *testing.T) {
	service := object("v1", "Service", "api", map[string]any{"spec": map[string]any{"selector": map[string]any{"app": "api"}}})
	slice := object("discovery.k8s.io/v1", "EndpointSlice", "api-x", map[string]any{"endpoints": []any{
		map[string]any{"addresses": []any{"10.0.0.1"}, "conditions": map[string]any{"ready": false}},
	}})
	slice.SetLabels(map[string]string{"kubernetes.io/service-name": "api"})
	cause := requireOne(t, diagnose(unhealthy(service), service, slice))
	if cause.Reason != "NoReadyEndpoints" || cause.Resource != idOf(service) {
		t.Fatalf("cause = %+v", cause)
	}

	ready := slice.DeepCopy()
	_ = unstructured.SetNestedSlice(ready.Object, []any{map[string]any{"addresses": []any{"10.0.0.1"}}}, "endpoints")
	if cause := requireOne(t, diagnose(unhealthy(service), service, *ready)); cause.Reason != "ReplicasUnavailable" {
		t.Fatalf("a Service with a ready endpoint was blamed: %+v", cause)
	}
}

func TestFailedJob(t *testing.T) {
	job := object("batch/v1", "Job", "migrate", map[string]any{"status": map[string]any{"conditions": []any{map[string]any{
		"type": "Failed", "status": "True", "reason": "BackoffLimitExceeded", "message": "Job has reached the specified backoff limit",
	}}}})
	results := []health.Result{{Resource: idOf(job), State: corev1alpha1.HealthStateDegraded, Reason: "JobFailed"}}
	cause := requireOne(t, diagnose(results, job))
	if cause.Reason != "JobFailed" || !strings.Contains(cause.Message, "BackoffLimitExceeded") {
		t.Fatalf("cause = %+v", cause)
	}

	failed := pod("migrate-x", job, map[string]any{}, map[string]any{"phase": "Failed", "containerStatuses": []any{map[string]any{
		"name": "migrate", "state": map[string]any{"terminated": map[string]any{"reason": "OOMKilled", "exitCode": int64(137), "message": containerOutput}},
	}}})
	cause = requireOne(t, diagnose(results, job, failed))
	if cause.Reason != "OOMKilled" || !strings.Contains(cause.Message, "exit code 137") || strings.Contains(cause.Message, containerOutput) {
		t.Fatalf("a failed Job Pod is the more specific cause: %+v", cause)
	}
	requireChain(t, cause, job, failed)
}

func TestSharedRootCauseIsReportedOnce(t *testing.T) {
	spec := envFromSecret("db", false)
	d := deployment("api", spec)
	rs := replicaSet(d, spec)
	objects := make([]unstructured.Unstructured, 0, 6)
	objects = append(objects, d, rs)
	for i := range 3 {
		objects = append(objects, pod(fmt.Sprintf("api-1-%d", i), rs, spec, waiting("CreateContainerConfigError", `secret "db" not found`, nil)))
	}
	worker := deployment("worker", spec)
	objects = append(objects, worker)
	cause := requireOne(t, diagnose(unhealthy(d, worker), objects...))
	if cause.Reason != "MissingSecret" || cause.Chain[2].Name != "api-1-0" {
		t.Fatalf("cause = %+v", cause)
	}
}

func TestCausesAreCappedAndDegradedFirst(t *testing.T) {
	objects := make([]unstructured.Unstructured, 0, MaxCauses+5)
	results := make([]health.Result, 0, MaxCauses+5)
	for i := range MaxCauses + 5 {
		d := deployment(fmt.Sprintf("app-%02d", i), envFromSecret(fmt.Sprintf("secret-%02d", i), false))
		objects = append(objects, d)
		results = append(results, unhealthy(d)...)
	}
	results[12].State = corev1alpha1.HealthStateDegraded
	causes := diagnose(results, objects...)
	if len(causes) != MaxCauses {
		t.Fatalf("got %d causes, want %d", len(causes), MaxCauses)
	}
	if causes[0].Resource.Name != "secret-12" || causes[1].Resource.Name != "secret-00" {
		t.Fatalf("order = %v, %v", causes[0].Resource, causes[1].Resource)
	}
	reversed := make([]unstructured.Unstructured, len(objects))
	for i := range objects {
		reversed[len(objects)-1-i] = objects[i]
	}
	if again := diagnose(results, reversed...); !reflect.DeepEqual(again, causes) {
		t.Fatal("diagnosis depends on input order")
	}
}

func TestUnhealthyObjectWithoutEvidenceIsItsOwnCause(t *testing.T) {
	widget := object("example.com/v1", "Widget", "w", nil)
	results := []health.Result{
		{Resource: idOf(widget), State: corev1alpha1.HealthStateDegraded, Reason: "Stalled", Message: "token=abc123 " + strings.Repeat("x", 2*MaxMessage)},
		{Resource: idOf(object("v1", "ConfigMap", "fine", nil)), State: corev1alpha1.HealthStateHealthy},
		{Resource: idOf(object("v1", "ConfigMap", "later", nil)), State: corev1alpha1.HealthStateProgressing, Reason: "Not Found!"},
	}
	causes := diagnose(results, widget)
	if len(causes) != 2 {
		t.Fatalf("causes = %+v", causes)
	}
	if causes[0].Reason != "Stalled" || !reflect.DeepEqual(causes[0].Chain, []resource.ID{idOf(widget)}) {
		t.Fatalf("cause = %+v", causes[0])
	}
	if strings.Contains(causes[0].Message, "abc123") || len(causes[0].Message) > MaxMessage {
		t.Fatalf("message not redacted and bounded: %d bytes %q", len(causes[0].Message), causes[0].Message[:40])
	}
	if causes[1].Reason != "NotFound" {
		t.Fatalf("reason is not CamelCase: %q", causes[1].Reason)
	}
}

func TestFallbackCauseSaysWhatWasNotVisible(t *testing.T) {
	d := deployment("api", map[string]any{})
	g := graph.Build([]unstructured.Unstructured{d})
	g.Unread = []string{"could not list Pods: forbidden", "could not list ReplicaSets: forbidden"}
	cause := requireOne(t, Build(context.Background(), Input{Results: unhealthy(d), Graph: g, Objects: []unstructured.Unstructured{d}}))
	want := "0/1 replicas available; not visible: could not list Pods: forbidden; could not list ReplicaSets: forbidden"
	if !cause.Fallback || cause.Message != want {
		t.Fatalf("cause = %+v, want message %q", cause, want)
	}
}

func TestStatusConvertsCauses(t *testing.T) {
	spec := envFromSecret("db", false)
	d, rs, p := workload(spec, waiting("CreateContainerConfigError", "", nil))
	got := Status(diagnose(unhealthy(d), d, rs, p))
	if len(got) != 1 || got[0].Resource != (corev1alpha1.ResourceRef{APIVersion: "v1", Kind: "Secret", Namespace: ns, Name: "db"}) || len(got[0].Chain) != 4 || got[0].Chain[0].APIVersion != "apps/v1" {
		t.Fatalf("status = %+v", got)
	}
	if Status(nil) != nil {
		t.Fatal("no causes must clear the status")
	}
}

// layered returns layers of ConfigMaps in which every object lists every
// object of the layer above as an owner, as a tenant could build them, so
// the number of paths from the top grows exponentially with depth.
func layered(width, depth int) []unstructured.Unstructured {
	top := object("v1", "ConfigMap", "layer-0-0", nil)
	objects := []unstructured.Unstructured{top}
	above := []unstructured.Unstructured{top}
	for layer := 1; layer <= depth; layer++ {
		current := make([]unstructured.Unstructured, 0, width)
		for i := range width {
			obj := object("v1", "ConfigMap", fmt.Sprintf("layer-%d-%d", layer, i), nil)
			owners := make([]metav1.OwnerReference, 0, len(above))
			for _, owner := range above {
				owners = append(owners, metav1.OwnerReference{APIVersion: "v1", Kind: "ConfigMap", Name: owner.GetName(), UID: types.UID("uid-" + owner.GetName())})
			}
			obj.SetOwnerReferences(owners)
			current = append(current, obj)
		}
		objects = append(objects, current...)
		above = current
	}
	return objects
}

func TestLayeredOwnerFanOutIsExplainedOncePerNode(t *testing.T) {
	const width, depth = 12, 9
	objects := layered(width, depth)
	results := []health.Result{{Resource: idOf(objects[0]), State: corev1alpha1.HealthStateDegraded, Reason: "Stalled"}}
	done := make(chan struct{})
	var causes []Cause
	var visits int
	go func() {
		defer close(done)
		causes, visits = build(context.Background(), Input{Results: results, Graph: graph.Build(objects), Objects: objects}, visitBudget)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("diagnosing a layered owner fan-out did not finish")
	}
	// Each node is explained at most once per depth it is reached at.
	if limit := len(objects) * MaxChain; visits > limit {
		t.Fatalf("explained %d nodes, want at most %d", visits, limit)
	}
	if len(causes) != 1 || !causes[0].Fallback {
		t.Fatalf("causes = %+v", causes)
	}
}

func TestWalkStopsAtItsBudgetAndOnCancellation(t *testing.T) {
	objects := layered(3, 4)
	in := Input{
		Results: []health.Result{{Resource: idOf(objects[0]), State: corev1alpha1.HealthStateDegraded, Reason: "Stalled"}},
		Graph:   graph.Build(objects), Objects: objects,
	}
	causes, visits := build(context.Background(), in, 5)
	if visits > 6 || len(causes) != 1 || causes[0].Reason != "Stalled" {
		t.Fatalf("visits=%d causes=%+v", visits, causes)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	causes, visits = build(cancelled, in, visitBudget)
	if visits > 1 || len(causes) != 1 || causes[0].Reason != "Stalled" {
		t.Fatalf("after cancellation visits=%d causes=%+v", visits, causes)
	}
}
