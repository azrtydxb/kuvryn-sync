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
	"strings"
	"time"

	authenticationv1 "k8s.io/api/authentication/v1"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	corev1alpha1 "github.com/azrtydxb/kuvryn-sync/api/v1alpha1"
	"github.com/azrtydxb/kuvryn-sync/internal/revisionid"
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
// forged or edited. It records the requester of a rollback the same way.
type ApplicationCustomDefaulter struct {
	// Reader must not be cached: the recorded digest has to be the plan that
	// is current now, not one a stale cache still holds.
	Reader client.Reader
	Now    func() time.Time
}

var rollbackRecord = []string{
	corev1alpha1.RollbackRequestedByAnnotation,
	corev1alpha1.RollbackRequestedAtAnnotation,
	corev1alpha1.RollbackTargetHashAnnotation,
}

// rollbackKind is the kind a rollback request has: one that does not say is
// manual, as the controller records it.
func rollbackKind(annotations map[string]string) string {
	if kind := annotations[corev1alpha1.RollbackKindAnnotation]; kind != "" {
		return kind
	}
	return corev1alpha1.RollbackKindManual
}

// recordRollbackRequester records who requested a rollback, when, and the
// desired-state hash of the Revision they chose, whenever the target, the
// chosen Revision or the kind changes: that person decided to deploy that
// Revision, and on an Application with manual sync their request approves
// its plan while its desired state is unchanged. The chosen Revision must
// exist, belong to the Application and be of the rollback's source
// revision. On every other change the record is restored, so it cannot be
// forged; the controller recording a request's source or its default kind
// keeps it. It is removed with the request, and never recorded for an
// unauthenticated request.
func (d *ApplicationCustomDefaulter) recordRollbackRequester(ctx context.Context, user authenticationv1.UserInfo, obj *corev1alpha1.Application, old, annotations map[string]string) error {
	target := strings.TrimSpace(annotations[corev1alpha1.RollbackRevisionAnnotation])
	chosen := strings.TrimSpace(annotations[corev1alpha1.RollbackTargetRevisionAnnotation])
	changed := target != strings.TrimSpace(old[corev1alpha1.RollbackRevisionAnnotation]) ||
		chosen != strings.TrimSpace(old[corev1alpha1.RollbackTargetRevisionAnnotation]) ||
		rollbackKind(annotations) != rollbackKind(old)
	for _, key := range rollbackRecord {
		value, ok := old[key]
		if ok && !changed {
			annotations[key] = value
		} else {
			delete(annotations, key)
		}
	}
	if !changed {
		return nil
	}
	var hash string
	if chosen != "" {
		if target == "" {
			return apierrors.NewBadRequest(fmt.Sprintf("%s requires %s", corev1alpha1.RollbackTargetRevisionAnnotation, corev1alpha1.RollbackRevisionAnnotation))
		}
		revision := &corev1alpha1.Revision{}
		if err := d.Reader.Get(ctx, client.ObjectKey{Namespace: obj.Namespace, Name: chosen}, revision); err != nil {
			return apierrors.NewBadRequest(fmt.Sprintf("cannot roll back to Revision %q: %v", chosen, err))
		}
		if revision.Spec.ApplicationRef.Name != obj.Name {
			return apierrors.NewBadRequest(fmt.Sprintf("cannot roll back to Revision %q: it belongs to Application %q, not %q", chosen, revision.Spec.ApplicationRef.Name, obj.Name))
		}
		if revision.Spec.Source.Revision != target {
			return apierrors.NewBadRequest(fmt.Sprintf("cannot roll back to Revision %q: it is of source revision %q, not %q", chosen, revision.Spec.Source.Revision, target))
		}
		hash = revisionid.RollbackBinding(revision, obj)
	}
	if target == "" || !authenticated(user) {
		return nil
	}
	annotations[corev1alpha1.RollbackRequestedByAnnotation] = user.Username
	annotations[corev1alpha1.RollbackRequestedAtAnnotation] = d.Now().UTC().Format(time.RFC3339)
	if hash != "" {
		annotations[corev1alpha1.RollbackTargetHashAnnotation] = hash
	}
	return nil
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
	if err := d.recordRollbackRequester(ctx, req.UserInfo, obj, old.GetAnnotations(), annotations); err != nil {
		return err
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
