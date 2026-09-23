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

// RepositoryType identifies a desired-state source adapter.
type RepositoryType string

const (
	// RepositoryTypeGit fetches desired state from a Git repository.
	RepositoryTypeGit RepositoryType = "git"
)

// RepositoryState is the observed readiness of a Repository.
type RepositoryState string

const (
	RepositoryStateUnknown RepositoryState = "Unknown"
	RepositoryStateReady   RepositoryState = "Ready"
	RepositoryStateFailed  RepositoryState = "Failed"
)

// SyncState describes desired/live convergence separately from health.
type SyncState string

const (
	SyncStateUnknown          SyncState = "Unknown"
	SyncStateSynced           SyncState = "Synced"
	SyncStateOutOfSync        SyncState = "OutOfSync"
	SyncStateDrifted          SyncState = "Drifted"
	SyncStatePlanning         SyncState = "Planning"
	SyncStateAwaitingApproval SyncState = "AwaitingApproval"
	SyncStateApplying         SyncState = "Applying"
	SyncStatePruning          SyncState = "Pruning"
)

// HealthState describes operational health separately from sync state.
type HealthState string

const (
	HealthStateUnknown     HealthState = "Unknown"
	HealthStateProgressing HealthState = "Progressing"
	HealthStateHealthy     HealthState = "Healthy"
	HealthStateDegraded    HealthState = "Degraded"
	HealthStateSuspended   HealthState = "Suspended"
)

// ConflictPolicy controls how SSA ownership conflicts are handled.
type ConflictPolicy string

const (
	// ConflictPolicyFail keeps conflict handling conservative and explicit.
	ConflictPolicyFail ConflictPolicy = "fail"
	// ConflictPolicyAdopt takes ownership of fields another manager holds,
	// listing each field and its previous manager in the plan.
	ConflictPolicyAdopt ConflictPolicy = "adopt"
)

// RenderType identifies the renderer used for an Application source path.
type RenderType string

const (
	RenderTypeYAML      RenderType = "yaml"
	RenderTypeKustomize RenderType = "kustomize"
	RenderTypeHelm      RenderType = "helm"
)

// FailureAction controls what happens when a deployment fails.
type FailureAction string

const (
	FailureActionPause    FailureAction = "pause"
	FailureActionRollback FailureAction = "rollback"
)

// DeletionPolicy controls what Solder does when an Application is deleted.
type DeletionPolicy string

const (
	// DeletionPolicyOrphan preserves workloads by default.
	DeletionPolicyOrphan DeletionPolicy = "Orphan"
	// DeletionPolicyDeleteManagedResources is intentionally explicit and dangerous.
	DeletionPolicyDeleteManagedResources DeletionPolicy = "DeleteManagedResources"
)

// RevisionPhase is the lifecycle phase of an attempted deployment.
type RevisionPhase string

const (
	RevisionPhasePending          RevisionPhase = "Pending"
	RevisionPhasePlanning         RevisionPhase = "Planning"
	RevisionPhaseAwaitingApproval RevisionPhase = "AwaitingApproval"
	RevisionPhaseApplying         RevisionPhase = "Applying"
	RevisionPhaseObserving        RevisionPhase = "Observing"
	RevisionPhaseHealthy          RevisionPhase = "Healthy"
	RevisionPhaseFailed           RevisionPhase = "Failed"
	RevisionPhaseRollingBack      RevisionPhase = "RollingBack"
	RevisionPhaseRolledBack       RevisionPhase = "RolledBack"
	RevisionPhaseCancelled        RevisionPhase = "Cancelled"
)

// PlanAction classifies the change for a planned resource.
type PlanAction string

const (
	PlanActionCreate    PlanAction = "Create"
	PlanActionUpdate    PlanAction = "Update"
	PlanActionDelete    PlanAction = "Delete"
	PlanActionUnchanged PlanAction = "Unchanged"
)

// LocalObjectReference references an object in the same namespace.
type LocalObjectReference struct {
	// name is the referenced object name.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}

// SecretReference references a Secret in the same namespace as the owning object.
type SecretReference struct {
	// name is the Secret name.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}

// ResourceRef identifies a Kubernetes resource in a plan, graph, or status.
type ResourceRef struct {
	// apiVersion is the resource API version, such as apps/v1.
	// +kubebuilder:validation:MinLength=1
	APIVersion string `json:"apiVersion"`
	// kind is the Kubernetes resource kind.
	// +kubebuilder:validation:MinLength=1
	Kind string `json:"kind"`
	// namespace is empty for cluster-scoped resources.
	// +optional
	Namespace string `json:"namespace,omitempty"`
	// name is the resource name.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}

// PlanSummary is a bounded summary of planned resource actions.
type PlanSummary struct {
	// +kubebuilder:validation:Minimum=0
	Create int32 `json:"create,omitempty"`
	// +kubebuilder:validation:Minimum=0
	Update int32 `json:"update,omitempty"`
	// +kubebuilder:validation:Minimum=0
	Delete int32 `json:"delete,omitempty"`
	// +kubebuilder:validation:Minimum=0
	Unchanged int32 `json:"unchanged,omitempty"`
	// truncated is true when detailed plan output was bounded for etcd safety.
	// +optional
	Truncated bool `json:"truncated,omitempty"`
}

// PlanFieldChange captures a redacted, machine-readable field-level change.
type PlanFieldChange struct {
	// path identifies the changed field in normalized Kubernetes object form.
	// +kubebuilder:validation:MinLength=1
	Path string `json:"path"`
	// before is a safe string representation of the previous value.
	// +optional
	Before string `json:"before,omitempty"`
	// after is a safe string representation of the desired value.
	// +optional
	After string `json:"after,omitempty"`
	// redacted indicates that the real value was intentionally hidden.
	// +optional
	Redacted bool `json:"redacted,omitempty"`
}

// PlanConflict captures a server-side apply ownership conflict detected before mutation.
type PlanConflict struct {
	// path identifies the desired field that conflicts with another field manager.
	// +kubebuilder:validation:MinLength=1
	Path string `json:"path"`
	// manager is the non-Solder field manager currently owning the field.
	// +kubebuilder:validation:MinLength=1
	Manager string `json:"manager"`
	// policy records the conflict behavior that will be used by apply.
	// +kubebuilder:validation:Enum=fail;adopt
	Policy ConflictPolicy `json:"policy"`
}

// PlanResourceChange describes one resource in a deployment plan.
type PlanResourceChange struct {
	Resource ResourceRef `json:"resource"`
	// +kubebuilder:validation:Enum=Create;Update;Delete;Unchanged
	Action PlanAction `json:"action"`
	// destructive is true for deletes and other planned operations that remove live state.
	// +optional
	Destructive bool `json:"destructive,omitempty"`
	// +listType=atomic
	// +optional
	Changes []PlanFieldChange `json:"changes,omitempty"`
	// conflicts lists SSA ownership conflicts that fail by default before mutation.
	// +listType=atomic
	// +optional
	Conflicts []PlanConflict `json:"conflicts,omitempty"`
	// warnings carry conspicuous safety information such as high-risk prune candidates.
	// +listType=atomic
	// +optional
	Warnings []string `json:"warnings,omitempty"`
}

// RevisionPlan stores bounded plan details in Revision status.
type RevisionPlan struct {
	Summary PlanSummary `json:"summary,omitempty"`
	// digest identifies the complete plan and desired state; an approval is
	// valid only for the digest it was given for.
	// +optional
	Digest string `json:"digest,omitempty"`
	// resources is intentionally bounded by the planner before it is written.
	// +listType=atomic
	// +optional
	Resources []PlanResourceChange `json:"resources,omitempty"`
}

// FailurePolicy declares deterministic failure behavior.
type FailurePolicy struct {
	// +kubebuilder:validation:Enum=pause;rollback
	// +kubebuilder:default:=pause
	Action FailureAction `json:"action,omitempty"`
	// timeout bounds apply/health observation before the attempt fails.
	// +optional
	Timeout *metav1.Duration `json:"timeout,omitempty"`
	// maxAttempts bounds retry loops for a failed desired revision.
	// +kubebuilder:validation:Minimum=1
	// +optional
	MaxAttempts *int32 `json:"maxAttempts,omitempty"`
}
