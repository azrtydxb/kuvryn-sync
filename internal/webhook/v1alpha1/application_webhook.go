/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	authenticationv1 "k8s.io/api/authentication/v1"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	corev1alpha1 "github.com/azrtydxb/kuvryn-sync/api/v1alpha1"
)

// authenticated reports whether an admission request carries a real identity.
// Anonymous requests have a non-empty username (system:anonymous), so the
// username alone does not tell.
func authenticated(user authenticationv1.UserInfo) bool {
	return user.Username != "" && user.Username != anonymousUser && !slices.Contains(user.Groups, unauthenticatedGroup)
}

const (
	anonymousUser        = "system:anonymous"
	unauthenticatedGroup = "system:unauthenticated"
)

// SetupApplicationWebhookWithManager registers the webhook for Application in the manager.
func SetupApplicationWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &corev1alpha1.Application{}).
		WithDefaulter(&ApplicationCustomDefaulter{Reader: mgr.GetAPIReader(), Now: time.Now}).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-sync-kuvryn-io-v1alpha1-application,mutating=true,failurePolicy=fail,sideEffects=None,groups=sync.kuvryn.io,resources=applications,verbs=create;update,versions=v1alpha1,name=mapplication-v1alpha1.kb.io,admissionReviewVersions=v1

// ApplicationCustomDefaulter records manual approvals. An approval is
// requested by changing the approved Revision, or by setting
// ApproveDigestAnnotation to the plan digest the approver reviewed, which
// also re-approves the same Revision after its plan changed. It stamps the
// authenticated requester, the time, and the Revision's current plan digest;
// on every other change it restores those annotations, so they cannot be
// forged or edited.
type ApplicationCustomDefaulter struct {
	// Reader must not be cached: the recorded digest has to be the plan that
	// is current now, not one a stale cache still holds.
	Reader client.Reader
	Now    func() time.Time
}

var approvalRecord = []string{
	corev1alpha1.ApprovedByAnnotation,
	corev1alpha1.ApprovedAtAnnotation,
	corev1alpha1.ApprovedDigestAnnotation,
}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind Application.
func (d *ApplicationCustomDefaulter) Default(ctx context.Context, obj *corev1alpha1.Application) error {
	req, err := admission.RequestFromContext(ctx)
	if err != nil {
		return err
	}
	old := &corev1alpha1.Application{}
	if len(req.OldObject.Raw) > 0 {
		if err := json.Unmarshal(req.OldObject.Raw, old); err != nil {
			return err
		}
	}
	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	// The request is consumed here and never stored, so a later
	// read-modify-write cannot replay it as a fresh approval.
	requested, hasRequest := annotations[corev1alpha1.ApproveDigestAnnotation]
	delete(annotations, corev1alpha1.ApproveDigestAnnotation)
	obj.SetAnnotations(annotations)

	approved := annotations[corev1alpha1.ApprovedRevisionAnnotation]
	if !hasRequest && approved == old.GetAnnotations()[corev1alpha1.ApprovedRevisionAnnotation] {
		for _, key := range approvalRecord {
			if value, ok := old.GetAnnotations()[key]; ok {
				annotations[key] = value
			} else {
				delete(annotations, key)
			}
		}
		return nil
	}
	for _, key := range approvalRecord {
		delete(annotations, key)
	}
	if approved == "" {
		if hasRequest {
			return apierrors.NewBadRequest(fmt.Sprintf("%s requires %s", corev1alpha1.ApproveDigestAnnotation, corev1alpha1.ApprovedRevisionAnnotation))
		}
		return nil
	}
	if !authenticated(req.UserInfo) {
		return apierrors.NewForbidden(schema.GroupResource{Group: corev1alpha1.GroupVersion.Group, Resource: "applications"}, obj.Name, fmt.Errorf("an approval must come from an authenticated user"))
	}
	revision := &corev1alpha1.Revision{}
	if err := d.Reader.Get(ctx, client.ObjectKey{Namespace: obj.Namespace, Name: approved}, revision); err != nil {
		return apierrors.NewBadRequest(fmt.Sprintf("cannot approve Revision %q: %v", approved, err))
	}
	current := revision.Status.Plan.Digest
	if revision.Spec.ApplicationRef.Name != obj.Name || current == "" {
		return apierrors.NewBadRequest(fmt.Sprintf("Revision %q has no plan for Application %q to approve", approved, obj.Name))
	}
	if hasRequest && requested != current {
		return apierrors.NewBadRequest(fmt.Sprintf("the plan of Revision %q changed since it was reviewed (reviewed digest %q, current digest %q); review the plan again and re-approve", approved, requested, current))
	}
	annotations[corev1alpha1.ApprovedByAnnotation] = req.UserInfo.Username
	annotations[corev1alpha1.ApprovedAtAnnotation] = d.Now().UTC().Format(time.RFC3339)
	annotations[corev1alpha1.ApprovedDigestAnnotation] = current
	return nil
}
