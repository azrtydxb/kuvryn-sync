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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ImagePolicySpec selects the image an automation should run.
type ImagePolicySpec struct {
	// image is the image repository to scan, such as ghcr.io/acme/api.
	// +kubebuilder:validation:MinLength=1
	Image string `json:"image"`
	// secretRef names a kubernetes.io/dockerconfigjson Secret, labelled
	// solder.io/registry-credentials=true, used to read the registry.
	// +optional
	SecretRef *SecretReference `json:"secretRef,omitempty"`
	// interval is how often the registry is scanned.
	// +kubebuilder:default:="5m"
	// +optional
	Interval metav1.Duration `json:"interval,omitempty"`
	// policy decides which image is selected.
	Policy ImageSelectionPolicy `json:"policy"`
	// webhook lets a registry or CI trigger an immediate scan through
	// Solder's receiver at /hooks/imagepolicies/<namespace>/<name>.
	// +optional
	Webhook *ImagePolicyWebhook `json:"webhook,omitempty"`
}

// ImagePolicyWebhook configures push notifications for an ImagePolicy.
type ImagePolicyWebhook struct {
	// secretRef names a Secret whose `token` authenticates requests, as a
	// GitHub signature, a GitLab token, or an Authorization Bearer token.
	SecretRef SecretReference `json:"secretRef"`
}

// ImageSelectionPolicy chooses one tag; exactly one field must be set.
// +kubebuilder:validation:XValidation:rule="(has(self.semver) ? 1 : 0) + (has(self.tagPattern) ? 1 : 0) + (has(self.digest) ? 1 : 0) == 1",message="set exactly one of semver, tagPattern, or digest"
type ImageSelectionPolicy struct {
	// semver selects the highest tag within a semantic version range.
	// +optional
	Semver *SemverPolicy `json:"semver,omitempty"`
	// tagPattern selects the last tag matching a regular expression, in the
	// given order.
	// +optional
	TagPattern *TagPatternPolicy `json:"tagPattern,omitempty"`
	// digest follows the current digest of one fixed tag, such as main.
	// +optional
	Digest *DigestPolicy `json:"digest,omitempty"`
}

// SemverPolicy selects by semantic version.
type SemverPolicy struct {
	// range is a semver constraint such as ">=1.2.0 <2.0.0".
	// +kubebuilder:validation:MinLength=1
	Range string `json:"range"`
}

// TagPatternPolicy selects by regular expression.
type TagPatternPolicy struct {
	// regex matches candidate tags.
	// +kubebuilder:validation:MinLength=1
	Regex string `json:"regex"`
	// order sorts matching tags: alphabetical or numerical (by the first
	// capture group, or the whole tag). The last tag in order is selected.
	// +kubebuilder:validation:Enum=alphabetical;numerical
	// +kubebuilder:default:=alphabetical
	// +optional
	Order string `json:"order,omitempty"`
}

// DigestPolicy follows a mutable tag by digest.
type DigestPolicy struct {
	// tag is the tag whose current digest is selected.
	// +kubebuilder:validation:MinLength=1
	Tag string `json:"tag"`
}

// ImagePolicyStatus reports the selected image.
type ImagePolicyStatus struct {
	// latestTag is the selected tag.
	// +optional
	LatestTag string `json:"latestTag,omitempty"`
	// latestDigest is the selected tag's manifest digest.
	// +optional
	LatestDigest string `json:"latestDigest,omitempty"`
	// latestImage is the immutable reference image:tag@digest.
	// +optional
	LatestImage string `json:"latestImage,omitempty"`
	// lastScannedAt is when the registry was last read successfully.
	// +optional
	LastScannedAt *metav1.Time `json:"lastScannedAt,omitempty"`
	// observedGeneration is the latest metadata.generation processed.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// conditions represent the state of the policy.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Image",type=string,JSONPath=`.status.latestImage`
// +kubebuilder:printcolumn:name="Scanned",type=date,JSONPath=`.status.lastScannedAt`

// ImagePolicy scans an image repository and selects the image to run.
type ImagePolicy struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of ImagePolicy
	// +required
	Spec ImagePolicySpec `json:"spec"`

	// status defines the observed state of ImagePolicy
	// +optional
	Status ImagePolicyStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// ImagePolicyList contains a list of ImagePolicy
type ImagePolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []ImagePolicy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ImagePolicy{}, &ImagePolicyList{})
}
