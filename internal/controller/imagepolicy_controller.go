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

package controller

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"oras.land/oras-go/v2/registry/remote/auth"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	corev1alpha1 "github.com/azrtydxb/solder/api/v1alpha1"
	"github.com/azrtydxb/solder/internal/imagepolicy"
)

// RegistryCredentialsLabel marks a Secret that Solder may send to a registry.
const RegistryCredentialsLabel = "solder.io/registry-credentials"

// imagePolicyChanges admits spec edits and annotation changes, such as the
// receiver's scan request, but not the controller's own status writes: each
// scan records lastScannedAt, which would otherwise trigger another scan at
// once and ignore the interval.
var imagePolicyChanges = predicate.Or(predicate.GenerationChangedPredicate{}, predicate.AnnotationChangedPredicate{})

// ImagePolicyReconciler scans registries and records the image each
// ImagePolicy selects.
type ImagePolicyReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	Registry *imagepolicy.Registry
}

// +kubebuilder:rbac:groups=solder.io,resources=imagepolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=solder.io,resources=imagepolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=solder.io,resources=imagepolicies/finalizers,verbs=update

// Reconcile scans the policy's image repository and records the selected
// tag and digest, then waits for the next interval.
func (r *ImagePolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	policy := &corev1alpha1.ImagePolicy{}
	if err := r.Get(ctx, req.NamespacedName, policy); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	interval := policy.Spec.Interval.Duration
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	policy.Status.ObservedGeneration = policy.Generation
	image, tag, digest, err := r.scan(ctx, policy)
	if err != nil {
		apimeta.SetStatusCondition(&policy.Status.Conditions, metav1.Condition{Type: "Ready", Status: metav1.ConditionFalse, Reason: "ScanFailed", Message: safeMessage(err, "Image repository could not be scanned"), ObservedGeneration: policy.Generation})
		return ctrl.Result{RequeueAfter: interval}, r.Status().Update(ctx, policy)
	}
	if image != policy.Status.LatestImage {
		log.Info("Selected image", "imagePolicy", policy.Name, "image", image)
		if r.Recorder != nil {
			r.Recorder.Event(policy, corev1.EventTypeNormal, "ImageSelected", "Selected "+image)
		}
	}
	now := metav1.Now()
	policy.Status.LatestTag, policy.Status.LatestDigest, policy.Status.LatestImage = tag, digest, image
	policy.Status.LastScannedAt = &now
	apimeta.SetStatusCondition(&policy.Status.Conditions, metav1.Condition{Type: "Ready", Status: metav1.ConditionTrue, Reason: "Selected", Message: "Selected " + image, ObservedGeneration: policy.Generation})
	return ctrl.Result{RequeueAfter: interval}, r.Status().Update(ctx, policy)
}

func (r *ImagePolicyReconciler) scan(ctx context.Context, policy *corev1alpha1.ImagePolicy) (image, tag, digest string, err error) {
	cred := auth.EmptyCredential
	if ref := policy.Spec.SecretRef; ref != nil {
		secret := &corev1.Secret{}
		if err := r.Get(ctx, client.ObjectKey{Namespace: policy.Namespace, Name: ref.Name}, secret); err != nil {
			if apierrors.IsNotFound(err) {
				return "", "", "", fmt.Errorf("registry credentials Secret %s was not found", ref.Name)
			}
			return "", "", "", err
		}
		// Whoever writes a policy chooses the registry host, so only Secrets
		// explicitly marked as registry credentials may be sent to it.
		if secret.Labels[RegistryCredentialsLabel] != "true" {
			return "", "", "", fmt.Errorf("registry credentials Secret %s is not labelled %s=true", ref.Name, RegistryCredentialsLabel)
		}
		if cred, err = imagepolicy.Credentials(secret.Data[corev1.DockerConfigJsonKey], policy.Spec.Image); err != nil {
			return "", "", "", err
		}
	}
	tags := []string{}
	if policy.Spec.Policy.Digest == nil {
		if tags, err = r.Registry.Tags(ctx, policy.Spec.Image, cred); err != nil {
			return "", "", "", err
		}
	}
	if tag, err = imagepolicy.Select(policy.Spec.Policy, tags); err != nil {
		return "", "", "", err
	}
	if digest, err = r.Registry.Digest(ctx, policy.Spec.Image, tag, cred); err != nil {
		return "", "", "", err
	}
	return fmt.Sprintf("%s:%s@%s", policy.Spec.Image, tag, digest), tag, digest, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ImagePolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.Recorder == nil {
		r.Recorder = mgr.GetEventRecorderFor("imagepolicy-controller")
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1alpha1.ImagePolicy{}, builder.WithPredicates(imagePolicyChanges)).
		Named("imagepolicy").
		Complete(r)
}
