package console

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	corev1alpha1 "github.com/azrtydxb/kuvryn-sync/api/v1alpha1"
)

// TestApplicationsReportRevisionReadFailures fails if a Revision list that
// fails for any reason but Forbidden is hidden behind the condition-time
// fallback: a timeout or outage must surface as a read failure, not as stale
// "Last change" times.
func TestApplicationsReportRevisionReadFailures(t *testing.T) {
	for name, tc := range map[string]struct {
		err     error
		wantErr bool
	}{
		"forbidden falls back":  {apierrors.NewForbidden(schema.GroupResource{Group: "sync.kuvryn.io", Resource: "revisions"}, "", errors.New("no")), false},
		"server error surfaces": {apierrors.NewInternalError(errors.New("etcd down")), true},
		"timeout surfaces":      {apierrors.NewTimeoutError("slow", 1), true},
	} {
		reader := fake.NewClientBuilder().WithScheme(testScheme()).WithInterceptorFuncs(interceptor.Funcs{
			List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
				if _, ok := list.(*corev1alpha1.RevisionList); ok {
					return tc.err
				}
				return c.List(ctx, list, opts...)
			},
		}).Build()
		_, err := (&Server{}).applications(context.Background(), httptest.NewRequest("GET", "/api/applications?namespace=a", nil), reader)
		if (err != nil) != tc.wantErr {
			t.Errorf("%s: err = %v, want error %v", name, err, tc.wantErr)
		}
	}
}
