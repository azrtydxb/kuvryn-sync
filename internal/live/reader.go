package live

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/azrtydxb/solder/internal/resource"
)

// Result contains live objects keyed by desired identity.
type Result struct {
	Found   map[resource.ID]unstructured.Unstructured
	Missing []resource.ID
}

// Reader loads live Kubernetes objects for desired identities.
type Reader struct {
	Client client.Client
}

func (r Reader) Read(ctx context.Context, desired []unstructured.Unstructured) (Result, error) {
	result := Result{Found: map[resource.ID]unstructured.Unstructured{}}
	for _, obj := range desired {
		id, err := resource.FromObject(obj)
		if err != nil {
			return Result{}, err
		}
		live := &unstructured.Unstructured{}
		live.SetGroupVersionKind(schema.GroupVersionKind{Group: id.Group, Version: id.Version, Kind: id.Kind})
		err = r.Client.Get(ctx, client.ObjectKey{Namespace: id.Namespace, Name: id.Name}, live)
		if err != nil {
			if apierrors.IsNotFound(err) {
				result.Missing = append(result.Missing, id)
				continue
			}
			return Result{}, err
		}
		result.Found[id] = *live
	}
	return result, nil
}
