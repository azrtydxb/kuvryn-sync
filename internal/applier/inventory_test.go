package applier

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	corev1alpha1 "github.com/azrtydxb/kuvryn-sync/api/v1alpha1"
)

func TestListManagedNeverListsExcludedKinds(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	labels := map[string]string{ApplicationLabelKey: "web"}
	listed := []string{}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: "a", Name: "cfg", Labels: labels}},
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: "a", Name: "creds", Labels: labels}},
	).WithInterceptorFuncs(interceptor.Funcs{List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
		gvk, _ := c.GroupVersionKindFor(list)
		listed = append(listed, gvk.Kind)
		return c.List(ctx, list, opts...)
	}}).Build()
	app := &corev1alpha1.Application{ObjectMeta: metav1.ObjectMeta{Namespace: "a", Name: "web"}}
	app.Status.ManagedKinds = []corev1alpha1.ManagedKind{{APIVersion: "v1", Kind: "ConfigMap"}, {APIVersion: "v1", Kind: "Secret"}}
	secrets := func(gvk schema.GroupVersionKind) bool { return gvk.Group == "" && gvk.Kind == "Secret" }
	got, _, err := ListManaged(context.Background(), c, app, ListOptions{Exclude: secrets})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].GetName() != "cfg" {
		t.Fatalf("ListManaged = %v", got)
	}
	for _, kind := range listed {
		if kind == "SecretList" || kind == "Secret" {
			t.Fatalf("an excluded kind was listed: %v", listed)
		}
	}
}
