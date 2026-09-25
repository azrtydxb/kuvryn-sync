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

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// HealthCheckSpec defines CEL health rules for one kind.
type HealthCheckSpec struct {
	// group is the API group of the kind the rules apply to; empty for the
	// core group.
	// +optional
	Group string `json:"group,omitempty"`
	// kind is the kind the rules apply to.
	// +kubebuilder:validation:MinLength=1
	Kind string `json:"kind"`
	// rules are evaluated in order against each live object of the kind; the
	// first rule whose expression is true decides its health. When no rule
	// matches, Kuvryn Sync falls back to kstatus conventions.
	// +listType=atomic
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=16
	Rules []HealthRule `json:"rules"`
}

// HealthRule maps a CEL condition to a health state.
type HealthRule struct {
	// expression is a CEL expression over the live object, available as
	// `object`, that evaluates to a bool.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=1024
	Expression string `json:"expression"`
	// state is the health reported when the expression is true.
	// +kubebuilder:validation:Enum=Healthy;Progressing;Degraded
	State HealthState `json:"state"`
	// message is reported with the state.
	// +kubebuilder:validation:MaxLength=256
	// +optional
	Message string `json:"message,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Group",type=string,JSONPath=`.spec.group`
// +kubebuilder:printcolumn:name="Kind",type=string,JSONPath=`.spec.kind`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// HealthCheck defines how Kuvryn Sync judges the health of a kind that kstatus
// conventions cannot describe.
type HealthCheck struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the health rules.
	// +required
	Spec HealthCheckSpec `json:"spec"`
}

// +kubebuilder:object:root=true

// HealthCheckList contains a list of HealthCheck
type HealthCheckList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []HealthCheck `json:"items"`
}

func init() {
	SchemeBuilder.Register(&HealthCheck{}, &HealthCheckList{})
}
