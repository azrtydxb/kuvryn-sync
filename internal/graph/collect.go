package graph

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/azrtydxb/solder/internal/resource"
)

const (
	// CollectObjectLimit bounds the live descendants and dependencies Collect
	// reads.
	CollectObjectLimit = 500
	// CollectSelectorLimit bounds the label selectors whose ReplicaSets and
	// Pods are listed.
	CollectSelectorLimit = 20
	// CollectListLimit bounds the objects one list request returns.
	CollectListLimit = 100
	// CollectReferenceLimit bounds the referenced objects read one by one.
	CollectReferenceLimit = 100
)

// Collect reads, through reader, the live ReplicaSets and Pods the
// managed objects own or select, the EndpointSlices of managed Services, and
// the objects any of them refers to, and relates them all in a graph.
// Secrets, ConfigMaps and ServiceAccounts are read as metadata only. Reads
// that are forbidden or fail only leave objects out: a referenced object
// that could not be checked is marked unreadable rather than missing.
func Collect(ctx context.Context, reader client.Reader, namespace string, managed []unstructured.Unstructured) (Graph, []unstructured.Unstructured) {
	c := graphCollector{reader: reader, namespace: namespace, seen: map[types.UID]bool{}, budget: CollectObjectLimit}
	owners := map[types.UID]bool{}
	for _, obj := range managed {
		c.objects = append(c.objects, obj)
		c.seen[obj.GetUID()] = true
		owners[obj.GetUID()] = true
	}
	selectors := []labels.Selector{}
	deploymentSelectors := []labels.Selector{}
	services := []string{}
	for _, obj := range managed {
		gvk := obj.GroupVersionKind()
		if gvk.Group == "" && gvk.Kind == "Service" {
			services = append(services, obj.GetName())
		}
		if selector := podSelector(obj); selector != nil && len(selectors) < CollectSelectorLimit {
			selectors = append(selectors, selector)
			if gvk.Group == "apps" && gvk.Kind == "Deployment" {
				deploymentSelectors = append(deploymentSelectors, selector)
			}
		}
	}
	for _, selector := range deploymentSelectors {
		for _, rs := range c.list(ctx, schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "ReplicaSet"}, client.MatchingLabelsSelector{Selector: selector}) {
			if ownedBy(rs, owners) {
				owners[rs.GetUID()] = true
				c.add(rs)
			}
		}
	}
	for _, selector := range selectors {
		for _, pod := range c.list(ctx, schema.GroupVersionKind{Version: "v1", Kind: "Pod"}, client.MatchingLabelsSelector{Selector: selector}) {
			// A Pod belongs in the graph whether a managed workload owns it
			// or a managed Service selects it; the graph links it to either.
			c.add(pod)
		}
	}
	for _, service := range services {
		c.addAll(c.list(ctx, schema.GroupVersionKind{Group: "discovery.k8s.io", Version: "v1", Kind: "EndpointSlice"}, client.MatchingLabels{"kubernetes.io/service-name": service}))
	}
	// Referenced objects may refer to more, such as a claim to its volume.
	unreadable := []resource.ID{}
	checked := map[resource.ID]bool{}
	for range 3 {
		fetched := false
		for _, id := range Build(c.objects).Missing() {
			if checked[id] {
				continue
			}
			checked[id] = true
			if len(checked) > CollectReferenceLimit || c.budget <= 0 {
				unreadable = append(unreadable, id)
				continue
			}
			obj, found, readable := c.get(ctx, id)
			switch {
			case found:
				c.add(obj)
				fetched = true
			case !readable:
				unreadable = append(unreadable, id)
			}
		}
		if !fetched {
			break
		}
	}
	g := Build(c.objects)
	g.MarkUnreadable(unreadable...)
	return g, c.objects
}

type graphCollector struct {
	reader    client.Reader
	namespace string
	objects   []unstructured.Unstructured
	seen      map[types.UID]bool
	budget    int
}

func (c *graphCollector) add(obj unstructured.Unstructured) {
	if uid := obj.GetUID(); uid != "" {
		if c.seen[uid] {
			return
		}
		c.seen[uid] = true
	}
	c.objects = append(c.objects, obj)
	c.budget--
}

func (c *graphCollector) addAll(objs []unstructured.Unstructured) {
	for _, obj := range objs {
		c.add(obj)
	}
}

// list returns at most one bounded page of objects of a kind.
func (c *graphCollector) list(ctx context.Context, gvk schema.GroupVersionKind, opts ...client.ListOption) []unstructured.Unstructured {
	if c.budget <= 0 {
		return nil
	}
	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(gvk.GroupVersion().WithKind(gvk.Kind + "List"))
	opts = append(opts, client.InNamespace(c.namespace), client.Limit(int64(min(CollectListLimit, c.budget))))
	if err := c.reader.List(ctx, list, opts...); err != nil {
		logf.FromContext(ctx).V(1).Info("Could not list objects for diagnosis", "kind", gvk.Kind, "namespace", c.namespace, "reason", apierrors.ReasonForError(err))
		return nil
	}
	return list.Items
}

// get reads one referenced object, as metadata only for kinds whose content
// diagnosis does not need. readable is false when its existence could not
// be checked.
func (c *graphCollector) get(ctx context.Context, id resource.ID) (obj unstructured.Unstructured, found, readable bool) {
	gvk := schema.GroupVersionKind{Group: id.Group, Version: id.Version, Kind: id.Kind}
	key := client.ObjectKey{Namespace: id.Namespace, Name: id.Name}
	var err error
	if id.Group == "" && (id.Kind == "Secret" || id.Kind == "ConfigMap" || id.Kind == "ServiceAccount") {
		meta := &metav1.PartialObjectMetadata{}
		meta.SetGroupVersionKind(gvk)
		if err = c.reader.Get(ctx, key, meta); err == nil {
			raw, convErr := runtime.DefaultUnstructuredConverter.ToUnstructured(meta)
			if convErr != nil {
				return obj, false, false
			}
			obj = unstructured.Unstructured{Object: raw}
			obj.SetGroupVersionKind(gvk)
		}
	} else {
		obj.SetGroupVersionKind(gvk)
		err = c.reader.Get(ctx, key, &obj)
	}
	switch {
	case err == nil:
		return obj, true, true
	case apierrors.IsNotFound(err):
		return obj, false, true
	default:
		logf.FromContext(ctx).V(1).Info("Could not read referenced object for diagnosis", "kind", id.Kind, "name", id.Name, "reason", apierrors.ReasonForError(err))
		return obj, false, false
	}
}

// podSelector returns the label selector of the Pods a managed object runs
// or selects, or nil.
func podSelector(obj unstructured.Unstructured) labels.Selector {
	gvk := obj.GroupVersionKind()
	if gvk.Group == "" && gvk.Kind == "Service" {
		selector, _, _ := unstructured.NestedStringMap(obj.Object, "spec", "selector")
		if len(selector) == 0 {
			return nil
		}
		return labels.SelectorFromSet(selector)
	}
	if _, ok := PodSpec(obj); !ok || gvk.Kind == "Pod" || gvk.Kind == "CronJob" {
		return nil
	}
	raw, found, _ := unstructured.NestedMap(obj.Object, "spec", "selector")
	if !found {
		return nil
	}
	var selector metav1.LabelSelector
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(raw, &selector); err != nil {
		return nil
	}
	parsed, err := metav1.LabelSelectorAsSelector(&selector)
	if err != nil || parsed.Empty() {
		return nil
	}
	return parsed
}

func ownedBy(obj unstructured.Unstructured, owners map[types.UID]bool) bool {
	for _, owner := range obj.GetOwnerReferences() {
		if owners[owner.UID] {
			return true
		}
	}
	return false
}
