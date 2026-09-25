package prune

import (
	"fmt"

	"github.com/azrtydxb/kuvryn-sync/internal/applier"
	"github.com/azrtydxb/kuvryn-sync/internal/ordering"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const PruneAnnotationKey = "sync.kuvryn.io/prune"

// Policy controls prune eligibility.
type Policy struct {
	Application   string
	AllowHighRisk bool
}

// Plan splits live objects into eligible prune candidates, objects prune
// keeps on purpose, and objects it must not touch.
func Plan(live []unstructured.Unstructured, policy Policy) Result {
	result := Result{}
	for _, obj := range live {
		if reason := rejectReason(obj, policy); reason != "" {
			result.Rejected = append(result.Rejected, Rejected{Object: obj, Reason: reason})
			continue
		}
		if reason := skipReason(obj, policy); reason != "" {
			result.Skipped = append(result.Skipped, Rejected{Object: obj, Reason: reason})
			continue
		}
		result.Eligible = append(result.Eligible, obj)
	}
	result.Eligible = ordering.Prune(result.Eligible)
	return result
}

// Result describes prune eligibility without mutating cluster state.
type Result struct {
	Eligible []unstructured.Unstructured
	// Skipped are managed objects prune keeps: they opted out with
	// sync.kuvryn.io/prune: disabled, or are of a high-risk kind. Keeping them is
	// not a failure.
	Skipped []Rejected
	// Rejected are objects Solder cannot show it manages, which must never
	// be pruned.
	Rejected []Rejected
}

// Rejected records why an object is not pruned.
type Rejected struct {
	Object unstructured.Unstructured
	Reason string
}

func rejectReason(obj unstructured.Unstructured, policy Policy) string {
	if policy.Application == "" {
		return "application identity is required"
	}
	if obj.GetLabels()[applier.ApplicationLabelKey] != policy.Application {
		return "resource is not managed by this Application"
	}
	return ""
}

func skipReason(obj unstructured.Unstructured, policy Policy) string {
	if obj.GetAnnotations()[PruneAnnotationKey] == "disabled" {
		return "prune disabled by sync.kuvryn.io/prune annotation"
	}
	if highRisk(obj) && !policy.AllowHighRisk {
		return fmt.Sprintf("high-risk %s is never pruned automatically", obj.GetKind())
	}
	return ""
}

// highRisk reports kinds whose deletion loses data or other workloads'
// state, which prune keeps rather than deletes.
func highRisk(obj unstructured.Unstructured) bool {
	switch obj.GetKind() {
	case "Namespace", "CustomResourceDefinition", "PersistentVolumeClaim", "PersistentVolume", "Secret":
		return true
	default:
		return false
	}
}
