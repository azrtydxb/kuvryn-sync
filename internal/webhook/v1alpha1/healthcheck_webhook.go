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

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	corev1alpha1 "github.com/azrtydxb/kuvryn-sync/api/v1alpha1"
	"github.com/azrtydxb/kuvryn-sync/internal/health"
)

// SetupHealthCheckWebhookWithManager registers the webhook for HealthCheck in the manager.
func SetupHealthCheckWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &corev1alpha1.HealthCheck{}).
		WithValidator(&HealthCheckCustomValidator{}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-sync-kuvryn-io-v1alpha1-healthcheck,mutating=false,failurePolicy=fail,sideEffects=None,groups=sync.kuvryn.io,resources=healthchecks,verbs=create;update,versions=v1alpha1,name=vhealthcheck-v1alpha1.kb.io,admissionReviewVersions=v1

// HealthCheckCustomValidator rejects HealthChecks whose rules are not valid
// CEL returning a bool, so a broken rule never reaches reconciliation.
type HealthCheckCustomValidator struct{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type HealthCheck.
func (v *HealthCheckCustomValidator) ValidateCreate(_ context.Context, obj *corev1alpha1.HealthCheck) (admission.Warnings, error) {
	return nil, validateHealthCheck(obj)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type HealthCheck.
func (v *HealthCheckCustomValidator) ValidateUpdate(_ context.Context, _, newObj *corev1alpha1.HealthCheck) (admission.Warnings, error) {
	return nil, validateHealthCheck(newObj)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type HealthCheck.
func (v *HealthCheckCustomValidator) ValidateDelete(_ context.Context, _ *corev1alpha1.HealthCheck) (admission.Warnings, error) {
	return nil, nil
}

func validateHealthCheck(check *corev1alpha1.HealthCheck) error {
	var errs field.ErrorList
	rules := field.NewPath("spec", "rules")
	for i, rule := range check.Spec.Rules {
		if _, err := health.CompileRule(rule.Expression); err != nil {
			errs = append(errs, field.Invalid(rules.Index(i).Child("expression"), rule.Expression, err.Error()))
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return apierrors.NewInvalid(corev1alpha1.GroupVersion.WithKind("HealthCheck").GroupKind(), check.Name, errs)
}
