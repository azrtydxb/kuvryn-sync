package applier

import (
	"context"
	"fmt"

	corev1alpha1 "github.com/azrtydxb/solder/api/v1alpha1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	FieldManager                 = "solder"
	ApplicationLabelKey          = "solder.io/application"
	ApplicationNamespaceLabelKey = "solder.io/application-namespace"
	RevisionAnnotationKey        = "solder.io/revision"
)

// Applier mutates Kubernetes resources with server-side apply.
type Applier struct {
	Client               client.Client
	FieldManager         string
	ApplicationNamespace string
}

// Result summarizes an apply pass.
type Result struct {
	Applied int
}

// Apply applies each desired object idempotently using SSA and fail-conflict semantics.
func (a Applier) Apply(ctx context.Context, application, revision string, desired []unstructured.Unstructured, policy corev1alpha1.ConflictPolicy) (Result, error) {
	if a.Client == nil {
		return Result{}, fmt.Errorf("applier client is required")
	}
	if policy == "" {
		policy = corev1alpha1.ConflictPolicyFail
	}
	if policy != corev1alpha1.ConflictPolicyFail {
		return Result{}, fmt.Errorf("unsupported conflict policy %q", policy)
	}
	manager := a.FieldManager
	if manager == "" {
		manager = FieldManager
	}
	result := Result{}
	for i := range desired {
		obj := desired[i].DeepCopy()
		markManaged(obj, application, a.ApplicationNamespace, revision)
		if err := a.Client.Apply(ctx, client.ApplyConfigurationFromUnstructured(obj), client.FieldOwner(manager)); err != nil {
			return result, err
		}
		result.Applied++
	}
	return result, nil
}

func markManaged(obj *unstructured.Unstructured, application, applicationNamespace, revision string) {
	labels := obj.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	if application != "" {
		labels[ApplicationLabelKey] = application
	}
	if applicationNamespace != "" {
		labels[ApplicationNamespaceLabelKey] = applicationNamespace
	}
	obj.SetLabels(labels)
	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	if revision != "" {
		annotations[RevisionAnnotationKey] = revision
	}
	obj.SetAnnotations(annotations)
}
