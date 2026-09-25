package applier

import (
	"cmp"
	"context"
	"maps"
	"slices"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1alpha1 "github.com/azrtydxb/kuvryn-sync/api/v1alpha1"
)

// DefaultKinds are the kinds earlier releases always inventoried. They are
// listed for an Application that has not recorded status.managedKinds yet.
var DefaultKinds = []schema.GroupVersionKind{
	{Group: "", Version: "v1", Kind: "ConfigMap"},
	{Group: "", Version: "v1", Kind: "Secret"},
	{Group: "", Version: "v1", Kind: "Service"},
	{Group: "apps", Version: "v1", Kind: "Deployment"},
	{Group: "apps", Version: "v1", Kind: "StatefulSet"},
	{Group: "apps", Version: "v1", Kind: "DaemonSet"},
}

// InventoryKinds returns the kinds recorded in an Application's inventory.
func InventoryKinds(application *corev1alpha1.Application) []schema.GroupVersionKind {
	kinds := make([]schema.GroupVersionKind, 0, len(application.Status.ManagedKinds))
	for _, kind := range application.Status.ManagedKinds {
		if gv, err := schema.ParseGroupVersion(kind.APIVersion); err == nil {
			kinds = append(kinds, gv.WithKind(kind.Kind))
		}
	}
	return UnionKinds(kinds)
}

// UnionKinds returns the distinct kinds of all sets, sorted.
func UnionKinds(sets ...[]schema.GroupVersionKind) []schema.GroupVersionKind {
	seen := map[schema.GroupVersionKind]struct{}{}
	for _, set := range sets {
		for _, gvk := range set {
			seen[gvk] = struct{}{}
		}
	}
	return slices.SortedFunc(maps.Keys(seen), func(a, b schema.GroupVersionKind) int { return cmp.Compare(a.String(), b.String()) })
}

// ListOptions tunes ListManaged.
type ListOptions struct {
	// DesiredKinds are listed in addition to the Application's inventory.
	DesiredKinds []schema.GroupVersionKind
	// MetadataOnly reports kinds to list as metadata only, for callers that
	// need only their identity, such as Secrets whose data they must not read.
	MetadataOnly func(schema.GroupVersionKind) bool
}

// ListManaged inventories, in the destination namespace, the objects labelled
// as managed by application. Kinds the reader may not list are returned as
// skipped, since their objects can neither be found nor pruned.
func ListManaged(ctx context.Context, reader client.Reader, application *corev1alpha1.Application, opts ListOptions) ([]unstructured.Unstructured, []string, error) {
	kinds := UnionKinds(InventoryKinds(application), opts.DesiredKinds)
	if len(application.Status.ManagedKinds) == 0 {
		// Applications last synced before the inventory existed may still own
		// objects of the kinds earlier releases always inventoried.
		kinds = UnionKinds(kinds, DefaultKinds)
	}
	out := []unstructured.Unstructured{}
	skipped := []string{}
	for _, gvk := range kinds {
		items, err := listKind(ctx, reader, gvk, opts.MetadataOnly != nil && opts.MetadataOnly(gvk),
			client.InNamespace(application.DestinationNamespace()), client.MatchingLabels{ApplicationLabelKey: application.Name})
		if err != nil {
			if apimeta.IsNoMatchError(err) {
				// The kind no longer exists, and neither do its objects.
				continue
			}
			if apierrors.IsForbidden(err) {
				skipped = append(skipped, gvk.Kind)
				continue
			}
			return nil, nil, err
		}
		for _, item := range items {
			ownerNamespace := item.GetLabels()[ApplicationNamespaceLabelKey]
			if ownerNamespace == "" || ownerNamespace == application.Namespace {
				out = append(out, item)
			}
		}
	}
	return out, skipped, nil
}

func listKind(ctx context.Context, reader client.Reader, gvk schema.GroupVersionKind, metadataOnly bool, opts ...client.ListOption) ([]unstructured.Unstructured, error) {
	listGVK := gvk.GroupVersion().WithKind(gvk.Kind + "List")
	if !metadataOnly {
		list := &unstructured.UnstructuredList{}
		list.SetGroupVersionKind(listGVK)
		if err := reader.List(ctx, list, opts...); err != nil {
			return nil, err
		}
		return list.Items, nil
	}
	list := &metav1.PartialObjectMetadataList{}
	list.SetGroupVersionKind(listGVK)
	if err := reader.List(ctx, list, opts...); err != nil {
		return nil, err
	}
	out := make([]unstructured.Unstructured, 0, len(list.Items))
	for i := range list.Items {
		raw, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&list.Items[i])
		if err != nil {
			return nil, err
		}
		item := unstructured.Unstructured{Object: raw}
		item.SetGroupVersionKind(gvk)
		out = append(out, item)
	}
	return out, nil
}
