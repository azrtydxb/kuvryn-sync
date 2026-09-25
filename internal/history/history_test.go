package history

import (
	"testing"
	"time"

	corev1alpha1 "github.com/azrtydxb/kuvryn-sync/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestApplyRetentionKeepsNewestRevisions(t *testing.T) {
	revs := []corev1alpha1.Revision{rev("old", 1), rev("new", 3), rev("mid", 2)}
	retention := ApplyRetention(revs, 2)
	if len(retention.Keep) != 2 || retention.Keep[0].Name != "new" || retention.Keep[1].Name != "mid" {
		t.Fatalf("keep = %#v", names(retention.Keep))
	}
	if len(retention.Delete) != 1 || retention.Delete[0].Name != "old" {
		t.Fatalf("delete = %#v", names(retention.Delete))
	}
}

func TestLimitForUsesApplicationPolicyOrDefault(t *testing.T) {
	limit := int32(7)
	app := corev1alpha1.Application{Spec: corev1alpha1.ApplicationSpec{History: corev1alpha1.HistoryPolicy{Limit: &limit}}}
	if got := LimitFor(app, 20); got != 7 {
		t.Fatalf("limit = %d", got)
	}
	if got := LimitFor(corev1alpha1.Application{}, 0); got != 20 {
		t.Fatalf("default limit = %d", got)
	}
}

func rev(name string, unix int64) corev1alpha1.Revision {
	return corev1alpha1.Revision{ObjectMeta: metav1.ObjectMeta{Name: name, CreationTimestamp: metav1.NewTime(time.Unix(unix, 0))}}
}

func names(revs []corev1alpha1.Revision) []string {
	out := make([]string, len(revs))
	for i, rev := range revs {
		out[i] = rev.Name
	}
	return out
}
