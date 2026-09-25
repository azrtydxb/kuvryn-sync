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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	corev1alpha1 "github.com/azrtydxb/kuvryn-sync/api/v1alpha1"
)

func healthCheck(name string, expressions ...string) *corev1alpha1.HealthCheck {
	check := &corev1alpha1.HealthCheck{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec:       corev1alpha1.HealthCheckSpec{Group: "argoproj.io", Kind: "Rollout"},
	}
	for _, expression := range expressions {
		check.Spec.Rules = append(check.Spec.Rules, corev1alpha1.HealthRule{Expression: expression, State: corev1alpha1.HealthStateHealthy})
	}
	return check
}

var _ = Describe("HealthCheck Webhook", func() {
	validator := HealthCheckCustomValidator{}

	It("admits rules that compile to a bool", func() {
		check := healthCheck("valid", `object.status.phase == "Healthy"`, `has(object.status.ready) && object.status.ready`)
		Expect(validator.ValidateCreate(ctx, check)).Error().NotTo(HaveOccurred())
		Expect(validator.ValidateUpdate(ctx, check, check)).Error().NotTo(HaveOccurred())
	})

	It("rejects syntax errors and non-bool rules, naming each field", func() {
		check := healthCheck("invalid", `object.status.phase ==`, `object.status.phase + ""`)
		_, err := validator.ValidateCreate(ctx, check)
		Expect(apierrors.IsInvalid(err)).To(BeTrue())
		Expect(err.Error()).To(And(ContainSubstring("spec.rules[0].expression"), ContainSubstring("spec.rules[1].expression")))
		Expect(validator.ValidateUpdate(ctx, healthCheck("invalid", "true"), check)).Error().To(HaveOccurred())
	})

	It("is enforced by the API server at admission", func() {
		invalid := healthCheck("rejected-at-admission", `object.status.phase ==`)
		err := k8sClient.Create(ctx, invalid)
		Expect(apierrors.IsInvalid(err)).To(BeTrue(), "API server admitted an invalid HealthCheck: %v", err)

		valid := healthCheck("admitted", `object.status.phase == "Healthy"`)
		Expect(k8sClient.Create(ctx, valid)).To(Succeed())
		Expect(k8sClient.Delete(ctx, valid)).To(Succeed())
	})
})
