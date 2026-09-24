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
	ApplicationNamespace string
}

// Apply applies each desired object idempotently using server-side apply.
// The fail policy surfaces ownership conflicts; adopt takes the fields over.
func (a Applier) Apply(ctx context.Context, application, revision string, desired []unstructured.Unstructured, policy corev1alpha1.ConflictPolicy) error {
	if policy != corev1alpha1.ConflictPolicyFail && policy != corev1alpha1.ConflictPolicyAdopt {
		return fmt.Errorf("unsupported conflict policy %q", policy)
	}
	options := []client.ApplyOption{client.FieldOwner(FieldManager)}
	if policy == corev1alpha1.ConflictPolicyAdopt {
		options = append(options, client.ForceOwnership)
	}
	for i := range desired {
		obj := desired[i].DeepCopy()
		MarkManaged(obj, application, a.ApplicationNamespace, revision)
		if err := a.Client.Apply(ctx, client.ApplyConfigurationFromUnstructured(obj), options...); err != nil {
			return err
		}
	}
	return nil
}

// MarkManaged adds the labels and annotation Solder applies with every
// object. Planning marks desired objects the same way, so this metadata is
// never mistaken for drift.
func MarkManaged(obj *unstructured.Unstructured, application, applicationNamespace, revision string) {
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
