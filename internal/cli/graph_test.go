package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	corev1alpha1 "github.com/azrtydxb/kuvryn-sync/api/v1alpha1"
	"github.com/azrtydxb/kuvryn-sync/internal/applier"
)

// graphClient holds an Application whose Deployment runs a Pod that needs a
// Secret that does not exist, plus an unmanaged Deployment and one managed by
// the Application of the same name in another namespace.
func graphClient(t *testing.T, extra ...client.Object) client.Client {
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
	objects := make([]client.Object, 0, 6+len(extra))
	objects = append(objects,
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
	)
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(append(objects, extra...)...).WithStatusSubresource(&corev1alpha1.Application{}).Build()
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

func TestGraphReadsManagedSecretsAsMetadataOnly(t *testing.T) {
	managed := map[string]string{applier.ApplicationLabelKey: "payments", applier.ApplicationNamespaceLabelKey: "default"}
	base := graphClient(t, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "api-token", Namespace: "payments", UID: "s", Labels: managed},
		Data:       map[string][]byte{"token": []byte("do-not-read")},
	}, &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "api-settings", Namespace: "payments", UID: "c", Labels: managed},
		Data:       map[string]string{"k": "v"},
	})
	app := &corev1alpha1.Application{}
	if err := base.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "payments"}, app); err != nil {
		t.Fatal(err)
	}
	app.Status.ManagedKinds = append(app.Status.ManagedKinds, corev1alpha1.ManagedKind{APIVersion: "v1", Kind: "Secret"}, corev1alpha1.ManagedKind{APIVersion: "v1", Kind: "ConfigMap"})
	if err := base.Status().Update(context.Background(), app); err != nil {
		t.Fatal(err)
	}
	c := interceptor.NewClient(base.(client.WithWatch), interceptor.Funcs{
		List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
			if full, ok := list.(*unstructured.UnstructuredList); ok && (full.GetKind() == "SecretList" || full.GetKind() == "ConfigMapList") {
				t.Errorf("listed %s in full", full.GetKind())
			}
			return c.List(ctx, list, opts...)
		},
		Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			if full, ok := obj.(*unstructured.Unstructured); ok && (full.GetKind() == "Secret" || full.GetKind() == "ConfigMap") {
				t.Errorf("read %s %s in full", full.GetKind(), key.Name)
			}
			return c.Get(ctx, key, obj, opts...)
		},
	})
	var stdout bytes.Buffer
	if err := writeGraph(context.Background(), c, "default", "payments", "json", &stdout); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"v1/Secret/payments/api-token", "v1/ConfigMap/payments/api-settings"} {
		if !strings.Contains(stdout.String(), `"id": "`+id+`"`) {
			t.Errorf("graph lacks managed %s:\n%s", id, stdout.String())
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
		"digraph kuvryn_sync {",
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

// leakedCredential is built at run time so secret scanners do not flag the
// fixture.
var leakedCredential = "pass" + "word=" + "not-a-real-one"

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
					{Resource: secret, Reason: "MissingSecret", Message: "Secret payments/db does not exist; " + leakedCredential, Chain: []corev1alpha1.ResourceRef{
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

func TestDiagnoseShowsAFailingReadyCondition(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(&corev1alpha1.Application{
		ObjectMeta: metav1.ObjectMeta{Name: "payments", Namespace: "default"},
		Status: corev1alpha1.ApplicationStatus{
			Health:     corev1alpha1.ApplicationHealthStatus{State: corev1alpha1.HealthStateDegraded},
			Conditions: []metav1.Condition{{Type: "Ready", Status: metav1.ConditionFalse, Reason: "SourceFailure", Message: "Referenced Repository was not found"}},
		},
	}).Build()
	var stdout bytes.Buffer
	if err := diagnose(context.Background(), c, "default", "payments", &stdout); err != nil {
		t.Fatal(err)
	}
	want := "payments: health Degraded, sync Unknown\nReady: False: SourceFailure: Referenced Repository was not found\n"
	if stdout.String() != want {
		t.Fatalf("output = %q, want %q", stdout.String(), want)
	}
}

func TestDiagnoseReturnsRevisionReadErrors(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	base := fake.NewClientBuilder().WithScheme(scheme).WithObjects(&corev1alpha1.Application{ObjectMeta: metav1.ObjectMeta{Name: "payments", Namespace: "default"}}).Build()
	denied := apierrors.NewForbidden(schema.GroupResource{Group: "sync.kuvryn.io", Resource: "revisions"}, "", nil)
	c := interceptor.NewClient(base.(client.WithWatch), interceptor.Funcs{
		List: func(context.Context, client.WithWatch, client.ObjectList, ...client.ListOption) error { return denied },
	})
	var stdout bytes.Buffer
	if err := diagnose(context.Background(), c, "default", "payments", &stdout); !apierrors.IsForbidden(err) {
		t.Fatalf("err = %v, output %q", err, stdout.String())
	}
}

// Catches the failure printed twice: a rollout failure also sets Ready to the
// same reason and message.
func TestDiagnosePrintsARolloutFailureOnce(t *testing.T) {
	failure := &corev1alpha1.RevisionFailure{Reason: "HealthFailure", Message: "One or more resources are degraded"}
	app := corev1alpha1.Application{
		ObjectMeta: metav1.ObjectMeta{Name: "payments"},
		Status: corev1alpha1.ApplicationStatus{
			Health:     corev1alpha1.ApplicationHealthStatus{State: corev1alpha1.HealthStateDegraded},
			Sync:       corev1alpha1.ApplicationSyncStatus{State: corev1alpha1.SyncStateOutOfSync},
			Conditions: []metav1.Condition{{Type: "Ready", Status: metav1.ConditionFalse, Reason: failure.Reason, Message: failure.Message}},
		},
	}
	want := "payments: health Degraded, sync OutOfSync\nFailure: HealthFailure: One or more resources are degraded\n"
	if out := RenderDiagnosis(app, failure); out != want {
		t.Fatalf("output = %q, want %q", out, want)
	}

	// A Ready condition saying something else is still shown.
	app.Status.Conditions[0].Reason, app.Status.Conditions[0].Message = "RetryBlocked", "maxAttempts 1 reached"
	want = "payments: health Degraded, sync OutOfSync\nReady: False: RetryBlocked: maxAttempts 1 reached\nFailure: HealthFailure: One or more resources are degraded\n"
	if out := RenderDiagnosis(app, failure); out != want {
		t.Fatalf("output = %q, want %q", out, want)
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
