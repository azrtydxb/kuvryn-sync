package graph

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
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
)

func TestCollectGraphTreatsForbiddenReadsAsNotVisible(t *testing.T) {
	deployment := unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1", "kind": "Deployment",
		"metadata": map[string]any{"name": "api", "namespace": "payments", "uid": "d-uid"},
		"spec": map[string]any{
			"selector": map[string]any{"matchLabels": map[string]any{"app": "api"}},
			"template": map[string]any{"spec": map[string]any{
				"containers": []any{map[string]any{"name": "api", "envFrom": []any{
					map[string]any{"secretRef": map[string]any{"name": "hidden"}},
					map[string]any{"secretRef": map[string]any{"name": "absent"}},
					map[string]any{"configMapRef": map[string]any{"name": "settings"}},
				}}},
			}},
		},
	}}
	settings := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "settings", Namespace: "payments"}, Data: map[string]string{"k": "v"}}
	forbidden := func(kind string) error {
		return apierrors.NewForbidden(schema.GroupResource{Resource: kind}, "", nil)
	}
	gets := []string{}
	tenant := interceptor.NewClient(fake.NewClientBuilder().WithScheme(clientgoscheme.Scheme).WithObjects(settings).Build(), interceptor.Funcs{
		List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
			return forbidden("pods")
		},
		Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			if _, full := obj.(*unstructured.Unstructured); full {
				t.Errorf("read %s in full; only its metadata is needed", key.Name)
			}
			gets = append(gets, key.Name)
			if key.Name == "hidden" {
				return forbidden("secrets")
			}
			return c.Get(ctx, key, obj, opts...)
		},
	})
	g, objects := Collect(context.Background(), tenant, "payments", []unstructured.Unstructured{deployment})
	if len(objects) != 2 {
		t.Fatalf("objects = %d, want the Deployment and the ConfigMap", len(objects))
	}
	if _, found := objects[1].Object["data"]; found {
		t.Fatal("the ConfigMap's data was read")
	}
	nodes := map[string]string{}
	for _, node := range g.Nodes {
		switch {
		case node.Missing:
			nodes[node.ID.Name] = "missing"
		case node.Unreadable:
			nodes[node.ID.Name] = "unreadable"
		default:
			nodes[node.ID.Name] = "present"
		}
	}
	if nodes["hidden"] != "unreadable" || nodes["absent"] != "missing" || nodes["settings"] != "present" {
		t.Fatalf("nodes = %v", nodes)
	}
	if len(gets) != 3 {
		t.Fatalf("gets = %v", gets)
	}
}

func TestCollectReadsDescendantsEndpointsAndReferences(t *testing.T) {
	selector := &metav1.LabelSelector{MatchLabels: map[string]string{"app": "api"}}
	controller := func(kind, name, uid string) []metav1.OwnerReference {
		apiVersion := "apps/v1"
		if kind == "Pod" {
			apiVersion = "v1"
		}
		return []metav1.OwnerReference{{APIVersion: apiVersion, Kind: kind, Name: name, UID: types.UID(uid), Controller: ptr.To(true)}}
	}
	deployment := &appsv1.Deployment{
		TypeMeta:   metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"},
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "payments", UID: "d"},
		Spec: appsv1.DeploymentSpec{Selector: selector, Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "api"}},
			Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "api", EnvFrom: []corev1.EnvFromSource{
				{SecretRef: &corev1.SecretEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "db"}}},
			}}}, Volumes: []corev1.Volume{{Name: "data", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "data"}}}}},
		}},
	}
	service := &corev1.Service{
		TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Service"},
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "payments", UID: "s"},
		Spec:       corev1.ServiceSpec{Selector: map[string]string{"app": "api"}},
	}
	managed := []unstructured.Unstructured{toUnstructured(t, deployment), toUnstructured(t, service)}
	tenant := fake.NewClientBuilder().WithScheme(clientgoscheme.Scheme).WithObjects(
		&appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{Name: "api-1", Namespace: "payments", UID: "rs", Labels: map[string]string{"app": "api"}, OwnerReferences: controller("Deployment", "api", "d")}},
		// A ReplicaSet with matching labels that the Deployment does not own.
		&appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{Name: "stray", Namespace: "payments", UID: "stray", Labels: map[string]string{"app": "api"}}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "api-1-a", Namespace: "payments", UID: "p", Labels: map[string]string{"app": "api"}, OwnerReferences: controller("ReplicaSet", "api-1", "rs")}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "payments", UID: "w", Labels: map[string]string{"app": "web"}}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "elsewhere", Namespace: "other", UID: "e", Labels: map[string]string{"app": "api"}}},
		&discoveryv1.EndpointSlice{ObjectMeta: metav1.ObjectMeta{Name: "api-x", Namespace: "payments", UID: "es", Labels: map[string]string{"kubernetes.io/service-name": "api"}}, AddressType: discoveryv1.AddressTypeIPv4},
		&corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "data", Namespace: "payments", UID: "pvc"}, Spec: corev1.PersistentVolumeClaimSpec{VolumeName: "pv-data"}},
		&corev1.PersistentVolume{ObjectMeta: metav1.ObjectMeta{Name: "pv-data", UID: "pv"}},
	).Build()

	g, objects := Collect(context.Background(), tenant, "payments", managed)
	names := map[string]bool{}
	for _, obj := range objects {
		names[obj.GetKind()+"/"+obj.GetName()] = true
	}
	for _, want := range []string{"ReplicaSet/api-1", "Pod/api-1-a", "EndpointSlice/api-x", "PersistentVolumeClaim/data", "PersistentVolume/pv-data"} {
		if !names[want] {
			t.Errorf("Collect did not read %s: %v", want, names)
		}
	}
	for _, unwanted := range []string{"ReplicaSet/stray", "Pod/web", "Pod/elsewhere"} {
		if names[unwanted] {
			t.Errorf("Collect read unrelated %s", unwanted)
		}
	}
	if missing := g.Missing(); len(missing) != 1 || missing[0].Kind != "Secret" {
		t.Fatalf("missing = %v, want only the Secret", missing)
	}
}

func toUnstructured(t *testing.T, obj any) unstructured.Unstructured {
	t.Helper()
	raw, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		t.Fatal(err)
	}
	return unstructured.Unstructured{Object: raw}
}
