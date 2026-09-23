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

// ApplicationSpec defines the desired state of Application.
type ApplicationSpec struct {
	// source identifies and renders desired Kubernetes objects.
	Source ApplicationSource `json:"source"`
	// destination constrains where namespaced resources are applied.
	Destination ApplicationDestination `json:"destination"`
	// sync controls planning, mutation, pruning, and drift repair.
	// +optional
	Sync SyncPolicy `json:"sync,omitempty"`
	// strategy controls deployment and failure behavior.
	// +optional
	Strategy DeploymentStrategy `json:"strategy,omitempty"`
	// health controls rollout observation.
	// +optional
	Health HealthPolicy `json:"health,omitempty"`
	// history controls bounded Revision retention.
	// +optional
	History HistoryPolicy `json:"history,omitempty"`
	// deletionPolicy controls whether workloads are preserved when the
	// Application is deleted. The default is Orphan.
	// +kubebuilder:validation:Enum=Orphan;DeleteManagedResources
	// +kubebuilder:default:=Orphan
	// +optional
	DeletionPolicy DeletionPolicy `json:"deletionPolicy,omitempty"`
	// suspend stops Solder from mutating managed resources while retaining status.
	// +optional
	Suspend bool `json:"suspend,omitempty"`
}

// ApplicationSource selects the desired state to render.
type ApplicationSource struct {
	// repositoryRef references a Repository in the Application namespace.
	RepositoryRef LocalObjectReference `json:"repositoryRef"`
	// revision is a branch, tag, or exact commit. When empty, the Repository
	// default revision is used.
	// +optional
	Revision string `json:"revision,omitempty"`
	// path is the repository-relative path containing desired state.
	// +optional
	Path string `json:"path,omitempty"`
	// render configures desired-state rendering.
	Render RenderSpec `json:"render"`
}

// RenderSpec configures the renderer used for desired state.
type RenderSpec struct {
	// type identifies the renderer.
	// +kubebuilder:validation:Enum=yaml;kustomize;helm
	Type RenderType `json:"type"`
	// helm configures Helm rendering when type is helm.
	// +optional
	Helm *HelmRenderSpec `json:"helm,omitempty"`
}

// HelmRenderSpec configures Helm rendering.
type HelmRenderSpec struct {
	// releaseName is the Helm release name used for template rendering.
	// +kubebuilder:validation:MaxLength=53
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`
	// +optional
	ReleaseName string `json:"releaseName,omitempty"`
	// valuesFiles are repository-relative values files.
	// +listType=atomic
	// +optional
	ValuesFiles []string `json:"valuesFiles,omitempty"`
}

// ApplicationDestination constrains where desired objects are applied.
type ApplicationDestination struct {
	// namespace is the default namespace for namespaced desired resources.
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// SyncPolicy controls sync behavior.
type SyncPolicy struct {
	// automatic allows Solder to apply approved plans without a separate command.
	// +optional
	Automatic bool `json:"automatic,omitempty"`
	// prune allows deletion of previously managed objects no longer in desired state.
	// +optional
	Prune bool `json:"prune,omitempty"`
	// selfHeal allows Solder to correct managed drift.
	// +optional
	SelfHeal bool `json:"selfHeal,omitempty"`
	// conflictPolicy controls SSA ownership conflict behavior.
	// +kubebuilder:validation:Enum=fail
	// +kubebuilder:default:=fail
	// +optional
	ConflictPolicy ConflictPolicy `json:"conflictPolicy,omitempty"`
}

// DeploymentStrategy controls rollout behavior.
type DeploymentStrategy struct {
	// type names the deployment strategy. v0.1 supports rolling semantics.
	// +kubebuilder:validation:Enum=rolling
	// +kubebuilder:default:=rolling
	// +optional
	Type string `json:"type,omitempty"`
	// failurePolicy controls deterministic failure handling.
	// +optional
	FailurePolicy FailurePolicy `json:"failurePolicy,omitempty"`
}

// HealthPolicy controls health observation.
type HealthPolicy struct {
	// timeout bounds rollout health observation.
	// +optional
	Timeout *metav1.Duration `json:"timeout,omitempty"`
}

// HistoryPolicy controls bounded Revision retention.
type HistoryPolicy struct {
	// limit is the maximum number of Revisions retained for this Application.
	// +kubebuilder:validation:Minimum=1
	// +optional
	Limit *int32 `json:"limit,omitempty"`
}

// ApplicationStatus defines the observed state of Application.
type ApplicationStatus struct {
	// state is the high-level health state displayed by kubectl and the CLI.
	// +kubebuilder:validation:Enum=Unknown;Progressing;Healthy;Degraded;Suspended
	// +optional
	State HealthState `json:"state,omitempty"`
	// desiredRevision is the source revision Git currently asks Solder to run.
	// +optional
	DesiredRevision string `json:"desiredRevision,omitempty"`
	// deployedRevision is the source revision currently deployed after rollback.
	// +optional
	DeployedRevision string `json:"deployedRevision,omitempty"`
	// sync reports desired/live convergence independent from health.
	// +optional
	Sync ApplicationSyncStatus `json:"sync,omitempty"`
	// health reports operational health independent from sync.
	// +optional
	Health ApplicationHealthStatus `json:"health,omitempty"`
	// resources summarizes managed resource health.
	// +optional
	Resources ResourceHealthSummary `json:"resources,omitempty"`
	// observedGeneration is the latest metadata.generation processed.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// conditions represent the current state of the Application resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// ApplicationSyncStatus reports desired/live convergence.
type ApplicationSyncStatus struct {
	// +kubebuilder:validation:Enum=Unknown;Synced;OutOfSync;Drifted;Planning;AwaitingApproval;Applying;Pruning
	// +optional
	State SyncState `json:"state,omitempty"`
}

// ApplicationHealthStatus reports rollout health.
type ApplicationHealthStatus struct {
	// +kubebuilder:validation:Enum=Unknown;Progressing;Healthy;Degraded;Suspended
	// +optional
	State HealthState `json:"state,omitempty"`
}

// ResourceHealthSummary is a bounded count of managed resource health.
type ResourceHealthSummary struct {
	// +kubebuilder:validation:Minimum=0
	Total int32 `json:"total,omitempty"`
	// +kubebuilder:validation:Minimum=0
	Healthy int32 `json:"healthy,omitempty"`
	// +kubebuilder:validation:Minimum=0
	Progressing int32 `json:"progressing,omitempty"`
	// +kubebuilder:validation:Minimum=0
	Degraded int32 `json:"degraded,omitempty"`
	// +kubebuilder:validation:Minimum=0
	Unknown int32 `json:"unknown,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=app;apps
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`
// +kubebuilder:printcolumn:name="Sync",type=string,JSONPath=`.status.sync.state`
// +kubebuilder:printcolumn:name="Health",type=string,JSONPath=`.status.health.state`
// +kubebuilder:printcolumn:name="Desired",type=string,JSONPath=`.status.desiredRevision`
// +kubebuilder:printcolumn:name="Deployed",type=string,JSONPath=`.status.deployedRevision`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Application is the main user-facing GitOps deployment abstraction.
type Application struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is standard Kubernetes object metadata.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired deployment behavior.
	// +required
	Spec ApplicationSpec `json:"spec"`

	// status defines observed sync, health, and revision state.
	// +optional
	Status ApplicationStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// ApplicationList contains a list of Application.
type ApplicationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []Application `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Application{}, &ApplicationList{})
}
