package prune

import (
	"fmt"

	"github.com/azrtydxb/solder/internal/applier"
	"github.com/azrtydxb/solder/internal/ordering"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const PruneAnnotationKey = "solder.io/prune"

// Policy controls prune eligibility.
type Policy struct {
	Application   string
	AllowHighRisk bool
}

// Plan splits live objects into eligible prune candidates and rejected resources with reasons.
func Plan(live []unstructured.Unstructured, policy Policy) Result {
	result := Result{}
	for _, obj := range live {
		if reason := rejectReason(obj, policy); reason != "" {
			result.Rejected = append(result.Rejected, Rejected{Object: obj, Reason: reason})
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
	Rejected []Rejected
}

// Rejected records why an object cannot be pruned.
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
	if obj.GetAnnotations()[PruneAnnotationKey] == "disabled" {
		return "prune disabled by solder.io/prune annotation"
	}
	if highRisk(obj) && !policy.AllowHighRisk {
		return fmt.Sprintf("high-risk %s prune requires policy approval", obj.GetKind())
	}
	return ""
}

func highRisk(obj unstructured.Unstructured) bool {
	switch obj.GetKind() {
	case "Namespace", "CustomResourceDefinition", "PersistentVolumeClaim", "PersistentVolume", "Secret":
		return true
	default:
		return false
	}
}
