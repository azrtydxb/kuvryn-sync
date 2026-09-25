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

// RepositorySpec defines the desired state of Repository.
type RepositorySpec struct {
	// type selects the desired-state source adapter.
	// +kubebuilder:validation:Enum=git
	// +kubebuilder:default:=git
	Type RepositoryType `json:"type,omitempty"`
	// git configures a Git desired-state source.
	// +optional
	Git *GitRepositorySpec `json:"git,omitempty"`
	// applicationConfigPaths are repository-relative .ksync.yaml files that
	// declare Applications for this Repository. When empty, Kuvryn Sync reads
	// .ksync.yaml from the repository root.
	// +listType=atomic
	// +optional
	ApplicationConfigPaths []string `json:"applicationConfigPaths,omitempty"`
	// applicationServiceAccountName is the service account that Applications
	// discovered from .ksync.yaml run as. Discovered Applications may only
	// name this account; when it is empty they may not set serviceAccountName
	// and use the controller's default service account.
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`
	// +optional
	ApplicationServiceAccountName string `json:"applicationServiceAccountName,omitempty"`
	// pollInterval controls source polling when no external wake-up signal exists.
	// +optional
	PollInterval *metav1.Duration `json:"pollInterval,omitempty"`
	// webhook lets GitHub or GitLab push events trigger an immediate fetch
	// through Solder's webhook receiver at /hooks/<namespace>/<name>.
	// +optional
	Webhook *RepositoryWebhook `json:"webhook,omitempty"`
	// imageUpdate commits the images selected by ImagePolicies in this
	// namespace back to the repository, wherever a file carries a marker such
	// as `# {"$imagepolicy": "<namespace>:<policy>"}`.
	// +optional
	ImageUpdate *ImageUpdateSpec `json:"imageUpdate,omitempty"`
}

// ImageUpdateSpec configures image write-back commits.
type ImageUpdateSpec struct {
	// secretRef names a Secret, labelled sync.kuvryn.io/git-credentials=true, with
	// credentials allowed to push (same keys as spec.git.auth).
	SecretRef SecretReference `json:"secretRef"`
	// branch receives the commits; it defaults to spec.git.revision.
	// +optional
	Branch string `json:"branch,omitempty"`
	// path limits which repository-relative directory is scanned for markers.
	// +optional
	Path string `json:"path,omitempty"`
	// authorName and authorEmail sign the commits.
	// +kubebuilder:default:="Kuvryn Sync"
	// +optional
	AuthorName string `json:"authorName,omitempty"`
	// +kubebuilder:default:="kuvryn-sync@localhost"
	// +optional
	AuthorEmail string `json:"authorEmail,omitempty"`
}

// RepositoryWebhook configures push webhooks for a Repository.
type RepositoryWebhook struct {
	// secretRef names a Secret in the Repository namespace whose `token` is the
	// webhook secret configured on GitHub (HMAC) or GitLab (token).
	SecretRef SecretReference `json:"secretRef"`
}

// GitRepositorySpec configures a Git desired-state source.
type GitRepositorySpec struct {
	// url is the Git remote URL. HTTPS and SSH are supported by the first API.
	// +kubebuilder:validation:MinLength=1
	URL string `json:"url"`
	// revision is the default branch, tag, or exact commit used by Applications
	// that do not specify their own source revision.
	// +optional
	Revision string `json:"revision,omitempty"`
	// auth references credentials for private repositories.
	// +optional
	Auth *GitAuthSpec `json:"auth,omitempty"`
}

// GitAuthSpec references source credentials without exposing their values.
type GitAuthSpec struct {
	// secretRef references a Secret in the Repository namespace.
	// +optional
	SecretRef *SecretReference `json:"secretRef,omitempty"`
}

// RepositoryStatus defines the observed state of Repository.
type RepositoryStatus struct {
	// state is the high-level observed readiness of the source.
	// +kubebuilder:validation:Enum=Unknown;Ready;Failed
	// +optional
	State RepositoryState `json:"state,omitempty"`
	// observedRevision is the latest resolved source revision Solder observed.
	// +optional
	ObservedRevision string `json:"observedRevision,omitempty"`
	// lastFetchedAt records the last successful source fetch/inspection time.
	// +optional
	LastFetchedAt *metav1.Time `json:"lastFetchedAt,omitempty"`
	// conditions represent the current state of the Repository resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=repo;repos
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`
// +kubebuilder:printcolumn:name="Revision",type=string,JSONPath=`.status.observedRevision`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Repository is a desired-state source and its authentication configuration.
type Repository struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is standard Kubernetes object metadata.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired source configuration.
	// +required
	Spec RepositorySpec `json:"spec"`

	// status defines the observed source state.
	// +optional
	Status RepositoryStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// RepositoryList contains a list of Repository.
type RepositoryList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []Repository `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Repository{}, &RepositoryList{})
}
