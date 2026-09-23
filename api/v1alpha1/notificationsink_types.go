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

// NotificationSinkType selects how a notification is delivered.
// +kubebuilder:validation:Enum=webhook;slack
type NotificationSinkType string

const (
	// NotificationSinkWebhook POSTs a JSON body signed with HMAC-SHA256.
	NotificationSinkWebhook NotificationSinkType = "webhook"
	// NotificationSinkSlack POSTs to a Slack incoming webhook.
	NotificationSinkSlack NotificationSinkType = "slack"
)

// NotificationSinkSpec defines where lifecycle notifications are delivered.
type NotificationSinkSpec struct {
	// type selects the delivery format.
	Type NotificationSinkType `json:"type"`
	// secretRef names a Secret in the sink's namespace holding `url` (an
	// https URL) and, for webhook sinks, `hmacKey` used to sign each body.
	SecretRef SecretReference `json:"secretRef"`
}

// +kubebuilder:object:root=true
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.type`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// NotificationSink is a destination for Application lifecycle notifications.
type NotificationSink struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the sink.
	// +required
	Spec NotificationSinkSpec `json:"spec"`
}

// +kubebuilder:object:root=true

// NotificationSinkList contains a list of NotificationSink
type NotificationSinkList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []NotificationSink `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NotificationSink{}, &NotificationSinkList{})
}
