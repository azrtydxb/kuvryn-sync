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

// RevisionSpec defines the immutable desired state of Revision.
type RevisionSpec struct {
	// applicationRef references the Application that owns this deployment attempt.
	ApplicationRef LocalObjectReference `json:"applicationRef"`
	// source records the exact desired-state source used for this attempt.
	Source RevisionSource `json:"source"`
	// provenance optionally links this desired-state change to build artifacts
	// and pipeline runs without coupling Solder to a specific CI system.
	// +optional
	Provenance *Provenance `json:"provenance,omitempty"`
	// desiredStateHash is a deterministic fingerprint of rendered desired state.
	// +optional
	DesiredStateHash string `json:"desiredStateHash,omitempty"`
}

// RevisionSource records resolved source identity for an attempted deployment.
type RevisionSource struct {
	// repositoryRef references the Repository used for this Revision.
	RepositoryRef LocalObjectReference `json:"repositoryRef"`
	// revision is the resolved source revision, usually a full Git commit SHA.
	// +kubebuilder:validation:MinLength=1
	Revision string `json:"revision"`
	// path is the repository-relative path rendered for this Revision.
	// +optional
	Path string `json:"path,omitempty"`
	// render records the render configuration used for this Revision.
	Render RenderSpec `json:"render"`
}

// Provenance links a Revision to source, artifact, and pipeline evidence.
type Provenance struct {
	// source identifies the product source that produced the artifact.
	// +optional
	Source *SourceProvenance `json:"source,omitempty"`
	// artifact identifies the immutable artifact promoted into desired state.
	// +optional
	Artifact *ArtifactProvenance `json:"artifact,omitempty"`
	// pipeline identifies the pipeline run that promoted desired state.
	// +optional
	Pipeline *PipelineProvenance `json:"pipeline,omitempty"`
}

// SourceProvenance identifies product source code.
type SourceProvenance struct {
	// +optional
	Repository string `json:"repository,omitempty"`
	// +optional
	Revision string `json:"revision,omitempty"`
}

// ArtifactProvenance identifies an immutable artifact.
type ArtifactProvenance struct {
	// +optional
	Type string `json:"type,omitempty"`
	// +optional
	URI string `json:"uri,omitempty"`
	// +optional
	Digest string `json:"digest,omitempty"`
}

// PipelineProvenance identifies a provider-neutral pipeline run.
type PipelineProvenance struct {
	// +optional
	Provider string `json:"provider,omitempty"`
	// +optional
	Run string `json:"run,omitempty"`
}

// RevisionStatus defines the observed state of Revision.
type RevisionStatus struct {
	// phase is the lifecycle state of this deployment attempt.
	// +kubebuilder:validation:Enum=Pending;Planning;AwaitingApproval;Applying;Observing;Healthy;Failed;RollingBack;RolledBack;Cancelled
	// +optional
	Phase RevisionPhase `json:"phase,omitempty"`
	// startedAt records when processing began.
	// +optional
	StartedAt *metav1.Time `json:"startedAt,omitempty"`
	// completedAt records when this Revision reached a terminal phase.
	// +optional
	CompletedAt *metav1.Time `json:"completedAt,omitempty"`
	// attempts counts reconciliation attempts for retry-loop protection.
	// +kubebuilder:validation:Minimum=0
	// +optional
	Attempts int32 `json:"attempts,omitempty"`
	// plan stores bounded, redacted deployment plan details.
	// +optional
	Plan RevisionPlan `json:"plan,omitempty"`
	// health summarizes health observed for this attempt.
	// +optional
	Health ResourceHealthSummary `json:"health,omitempty"`
	// previousRevision references the prior healthy Revision when known.
	// +optional
	PreviousRevision *LocalObjectReference `json:"previousRevision,omitempty"`
	// failure records the most recent deterministic failure classification.
	// +optional
	Failure *RevisionFailure `json:"failure,omitempty"`
	// approval records who approved this Revision's plan, when it was applied
	// through manual approval.
	// +optional
	Approval *RevisionApproval `json:"approval,omitempty"`
	// chartDigest is the sha256 of the Helm chart archive pulled from a chart
	// repository for this Revision.
	// +optional
	ChartDigest string `json:"chartDigest,omitempty"`
	// hooks reports the pre-sync and post-sync hooks run for this Revision.
	// +listType=atomic
	// +kubebuilder:validation:MaxItems=64
	// +optional
	Hooks []HookStatus `json:"hooks,omitempty"`
	// conditions represent the current state of the Revision resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// HookStatus is the state of one sync hook.
type HookStatus struct {
	// resource identifies the hook object.
	Resource ResourceRef `json:"resource"`
	// stage is PreSync or PostSync.
	Stage string `json:"stage"`
	// state is the hook's health: Progressing while it runs, Healthy once it
	// succeeded, Degraded when it failed.
	State HealthState `json:"state"`
	// message explains the state.
	// +optional
	Message string `json:"message,omitempty"`
}

// RevisionApproval is the audit record of a manual approval.
type RevisionApproval struct {
	// approvedBy is the authenticated user who approved the plan.
	ApprovedBy string `json:"approvedBy"`
	// approvedAt is when the approval was admitted.
	ApprovedAt metav1.Time `json:"approvedAt"`
	// planDigest is the plan digest the approval was given for.
	PlanDigest string `json:"planDigest"`
}

// RevisionFailure describes a machine-readable deployment failure.
type RevisionFailure struct {
	// reason is a stable category such as RenderFailure or HealthFailure.
	// +optional
	Reason string `json:"reason,omitempty"`
	// message is a concise human-readable explanation.
	// +optional
	Message string `json:"message,omitempty"`
	// resource identifies the affected resource when applicable.
	// +optional
	Resource *ResourceRef `json:"resource,omitempty"`
	// retryable indicates whether retrying the same desired revision can help.
	// +optional
	Retryable bool `json:"retryable,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=rev;revs
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Application",type=string,JSONPath=`.spec.applicationRef.name`
// +kubebuilder:printcolumn:name="Revision",type=string,JSONPath=`.spec.source.revision`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Revision is an auditable, bounded record of one deployment attempt.
type Revision struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is standard Kubernetes object metadata.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines immutable deployment attempt identity.
	// +required
	Spec RevisionSpec `json:"spec"`

	// status defines observed lifecycle, plan, and health state.
	// +optional
	Status RevisionStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// RevisionList contains a list of Revision.
type RevisionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []Revision `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Revision{}, &RevisionList{})
}
