package applier

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	corev1alpha1 "github.com/azrtydxb/kuvryn-sync/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	FieldManager                 = "kuvryn-sync"
	ApplicationLabelKey          = "sync.kuvryn.io/application"
	ApplicationNamespaceLabelKey = "sync.kuvryn.io/application-namespace"
	RevisionAnnotationKey        = "sync.kuvryn.io/revision"
	// LegacyFieldManager is the manager the API server records for the
	// fields an object already had when it was first server-side applied,
	// such as everything client-side apply or Helm set before Kuvryn Sync
	// took over. No controller writes as it.
	LegacyFieldManager = "before-first-apply"
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
		err := a.Client.Apply(ctx, client.ApplyConfigurationFromUnstructured(obj), options...)
		if err != nil && policy == corev1alpha1.ConflictPolicyFail && onlyLegacyConflicts(err) {
			// Fields the API server attributes to the legacy owner were set
			// before any server-side apply, such as by client-side apply or
			// Helm before Kuvryn Sync took over: take them over. The server
			// named every conflict, so no other manager's field is taken.
			err = a.Client.Apply(ctx, client.ApplyConfigurationFromUnstructured(obj), client.FieldOwner(FieldManager), client.ForceOwnership)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// onlyLegacyConflicts reports whether err is a server-side apply conflict
// whose every conflicting field is held by LegacyFieldManager alone.
func onlyLegacyConflicts(err error) bool {
	var status apierrors.APIStatus
	if !apierrors.IsConflict(err) || !errors.As(err, &status) || status.Status().Details == nil {
		return false
	}
	causes := status.Status().Details.Causes
	if len(causes) == 0 {
		return false
	}
	for _, cause := range causes {
		if cause.Type != metav1.CauseTypeFieldManagerConflict || !strings.Contains(cause.Message, strconv.Quote(LegacyFieldManager)) {
			return false
		}
	}
	return true
}

// MarkManaged adds the labels and annotation Kuvryn Sync applies with every
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
