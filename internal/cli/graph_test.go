package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	corev1alpha1 "github.com/azrtydxb/solder/api/v1alpha1"
	"github.com/azrtydxb/solder/internal/applier"
)

// graphClient holds an Application whose Deployment runs a Pod that needs a
// Secret that does not exist, plus an unmanaged Deployment and one managed by
// the Application of the same name in another namespace.
func graphClient(t *testing.T) client.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := corev1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	managed := map[string]string{applier.ApplicationLabelKey: "payments", applier.ApplicationNamespaceLabelKey: "default"}
	selector := &metav1.LabelSelector{MatchLabels: map[string]string{"app": "api"}}
	template := corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "api"}},
		Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "api", Image: "api:1", EnvFrom: []corev1.EnvFromSource{
			{SecretRef: &corev1.SecretEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "db"}}},
		}}}},
	}
	owner := func(apiVersion, kind, name, uid string) []metav1.OwnerReference {
		return []metav1.OwnerReference{{APIVersion: apiVersion, Kind: kind, Name: name, UID: types.UID(uid), Controller: ptr.To(true)}}
	}
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		&corev1alpha1.Application{
			ObjectMeta: metav1.ObjectMeta{Name: "payments", Namespace: "default"},
			Spec:       corev1alpha1.ApplicationSpec{Destination: corev1alpha1.ApplicationDestination{Namespace: "payments"}},
			Status:     corev1alpha1.ApplicationStatus{ManagedKinds: []corev1alpha1.ManagedKind{{APIVersion: "apps/v1", Kind: "Deployment"}}},
		},
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "payments", UID: "d", Labels: managed}, Spec: appsv1.DeploymentSpec{Selector: selector, Template: template}},
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "unmanaged", Namespace: "payments", UID: "u"}, Spec: appsv1.DeploymentSpec{Selector: selector, Template: template}},
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "foreign", Namespace: "payments", UID: "f", Labels: map[string]string{
			applier.ApplicationLabelKey: "payments", applier.ApplicationNamespaceLabelKey: "other",
		}}, Spec: appsv1.DeploymentSpec{Selector: selector, Template: template}},
		&appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{Name: "api-1", Namespace: "payments", UID: "rs", Labels: map[string]string{"app": "api"}, OwnerReferences: owner("apps/v1", "Deployment", "api", "d")}, Spec: appsv1.ReplicaSetSpec{Selector: selector, Template: template}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "api-1-a", Namespace: "payments", UID: "p", Labels: map[string]string{"app": "api"}, OwnerReferences: owner("apps/v1", "ReplicaSet", "api-1", "rs")}, Spec: template.Spec},
	).Build()
}

func TestGraphJSONFollowsTheApplicationsManagedObjects(t *testing.T) {
	var stdout bytes.Buffer
	if err := writeGraph(context.Background(), graphClient(t), "default", "payments", "json", &stdout); err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Nodes []struct {
			ID      string `json:"id"`
			Missing bool   `json:"missing"`
		} `json:"nodes"`
		Edges []struct {
			From string `json:"from"`
			To   string `json:"to"`
			Type string `json:"type"`
		} `json:"edges"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &doc); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, stdout.String())
	}
	nodes := map[string]bool{}
	for _, node := range doc.Nodes {
		nodes[node.ID] = node.Missing
	}
	want := map[string]bool{
		"apps/v1/Deployment/payments/api":   false,
		"apps/v1/ReplicaSet/payments/api-1": false,
		"v1/Pod/payments/api-1-a":           false,
		"v1/Secret/payments/db":             true,
	}
	for id, missing := range want {
		if got, ok := nodes[id]; !ok || got != missing {
			t.Errorf("node %s: present=%v missing=%v, want missing=%v", id, ok, got, missing)
		}
	}
	for _, other := range []string{"unmanaged", "foreign"} {
		if _, ok := nodes["apps/v1/Deployment/payments/"+other]; ok {
			t.Errorf("graph includes Deployment %s, which this Application does not manage", other)
		}
	}
	edges := map[string]bool{}
	for _, edge := range doc.Edges {
		edges[edge.From+" "+edge.Type+" "+edge.To] = true
	}
	for _, edge := range []string{
		"apps/v1/Deployment/payments/api Owns apps/v1/ReplicaSet/payments/api-1",
		"apps/v1/ReplicaSet/payments/api-1 Owns v1/Pod/payments/api-1-a",
		"v1/Pod/payments/api-1-a Uses v1/Secret/payments/db",
	} {
		if !edges[edge] {
			t.Errorf("missing edge %q in %v", edge, edges)
		}
	}
}

func TestGraphDOT(t *testing.T) {
	var stdout bytes.Buffer
	if err := writeGraph(context.Background(), graphClient(t), "default", "payments", "dot", &stdout); err != nil {
		t.Fatal(err)
	}
	dot := stdout.String()
	for _, want := range []string{
		"digraph solder {",
		`"apps/v1/Deployment/payments/api" -> "apps/v1/ReplicaSet/payments/api-1" [label="Owns"];`,
		`"v1/Secret/payments/db" [label="Secret\npayments/db", style=dashed, color=red, xlabel="missing"];`,
	} {
		if !strings.Contains(dot, want) {
			t.Errorf("DOT missing %q:\n%s", want, dot)
		}
	}
}

func TestGraphRejectsUnknownFormats(t *testing.T) {
	var stdout, stderr bytes.Buffer
	handled, code := Run(context.Background(), []string{"graph", "payments", "-o", "svg"}, &stdout, &stderr)
	if !handled || code != 1 || !strings.Contains(stderr.String(), "usage: solder graph") {
		t.Fatalf("handled=%v code=%d stderr=%s", handled, code, stderr.String())
	}
}

func TestDiagnosePrintsChains(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	ref := func(apiVersion, kind, name string) corev1alpha1.ResourceRef {
		return corev1alpha1.ResourceRef{APIVersion: apiVersion, Kind: kind, Namespace: "payments", Name: name}
	}
	secret := ref("v1", "Secret", "db")
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		&corev1alpha1.Application{
			ObjectMeta: metav1.ObjectMeta{Name: "payments", Namespace: "default"},
			Status: corev1alpha1.ApplicationStatus{
				Health: corev1alpha1.ApplicationHealthStatus{State: corev1alpha1.HealthStateDegraded},
				Sync:   corev1alpha1.ApplicationSyncStatus{State: corev1alpha1.SyncStateOutOfSync},
				Diagnosis: []corev1alpha1.DiagnosisCause{
					{Resource: secret, Reason: "MissingSecret", Message: "Secret payments/db does not exist; password=hunter2", Chain: []corev1alpha1.ResourceRef{
						ref("apps/v1", "Deployment", "api"), ref("apps/v1", "ReplicaSet", "api-1"), ref("v1", "Pod", "api-1-a"), secret,
					}},
					{Resource: ref("v1", "Pod", "worker-0"), Reason: "ImagePullBackOff", Chain: []corev1alpha1.ResourceRef{ref("apps/v1", "StatefulSet", "worker"), ref("v1", "Pod", "worker-0")}},
				},
			},
		},
		&corev1alpha1.Revision{
			ObjectMeta: metav1.ObjectMeta{Name: "payments-abc", Namespace: "default"},
			Spec:       corev1alpha1.RevisionSpec{ApplicationRef: corev1alpha1.LocalObjectReference{Name: "payments"}},
			Status:     corev1alpha1.RevisionStatus{Failure: &corev1alpha1.RevisionFailure{Reason: "HealthFailure", Message: "One or more resources are degraded"}},
		},
	).Build()
	var stdout bytes.Buffer
	if err := diagnose(context.Background(), c, "default", "payments", &stdout); err != nil {
		t.Fatal(err)
	}
	want := `payments: health Degraded, sync OutOfSync
Failure: HealthFailure: One or more resources are degraded
Causes (2):

1. MissingSecret  Secret/payments/db
   Secret payments/db does not exist; password=REDACTED
   Deployment/payments/api
   └─ ReplicaSet/payments/api-1
      └─ Pod/payments/api-1-a
         └─ Secret/payments/db

2. ImagePullBackOff  Pod/payments/worker-0
   StatefulSet/payments/worker
   └─ Pod/payments/worker-0
`
	if stdout.String() != want {
		t.Fatalf("diagnose output:\n%s\nwant:\n%s", stdout.String(), want)
	}
}

func TestDiagnoseWithoutFailure(t *testing.T) {
	out := RenderDiagnosis(corev1alpha1.Application{ObjectMeta: metav1.ObjectMeta{Name: "payments"}}, nil)
	if out != "payments: health Unknown, sync Unknown\nNo failure or unhealthy resource recorded\n" {
		t.Fatalf("output = %q", out)
	}
}

func TestHelpListsEveryCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if handled, code := Run(context.Background(), []string{"help"}, &stdout, &stderr); !handled || code != 0 {
		t.Fatalf("handled=%v code=%d", handled, code)
	}
	for _, command := range []string{"apps", "repos", "repo get", "get", "history", "revision", "plan", "diagnose", "graph", "drift", "sync", "rollback", "suspend", "resume", "install", "version"} {
		if !strings.Contains(stdout.String(), "  "+command+" ") {
			t.Errorf("help does not list %q", command)
		}
	}
}
