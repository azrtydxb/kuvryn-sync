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
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Manual approval annotations on an Application. Users set
// ApprovedRevisionAnnotation and, to approve a specific plan,
// ApproveDigestAnnotation; the admission webhook records the rest from the
// authenticated request and restores them on every other change.
const (
	// ApprovedRevisionAnnotation names the Revision being approved.
	ApprovedRevisionAnnotation = "sync.kuvryn.io/approved-revision"
	// ApproveDigestAnnotation requests an approval of the plan digest the
	// approver reviewed. The admission webhook rejects it when the Revision's
	// plan has since changed, and never stores it.
	ApproveDigestAnnotation = "sync.kuvryn.io/approve-digest"
	// ApprovedByAnnotation is the authenticated user who approved.
	ApprovedByAnnotation = "sync.kuvryn.io/approved-by"
	// ApprovedAtAnnotation is when the approval was admitted, in RFC 3339.
	ApprovedAtAnnotation = "sync.kuvryn.io/approved-at"
	// ApprovedDigestAnnotation is the plan digest the approval binds to.
	ApprovedDigestAnnotation = "sync.kuvryn.io/approved-digest"
)

// Rollback request annotations on an Application. `ksync rollback` and a
// rollback failure policy set all three; Kuvryn Sync removes them once the
// rollback completes or is abandoned.
const (
	// RollbackRevisionAnnotation is the source revision to roll back to.
	RollbackRevisionAnnotation = "sync.kuvryn.io/rollback-revision"
	// RollbackFromAnnotation is the source revision rolled back from. Once
	// the rollback completes, every Revision of that source revision is held:
	// Kuvryn Sync does not deploy it again until a new commit arrives.
	RollbackFromAnnotation = "sync.kuvryn.io/rollback-from"
	// RollbackKindAnnotation is RollbackKindManual or RollbackKindAutomatic.
	RollbackKindAnnotation = "sync.kuvryn.io/rollback-kind"
	// RollbackKindManual is a rollback a user requested.
	RollbackKindManual = "manual"
	// RollbackKindAutomatic is a rollback the failure policy started after a
	// failed rollout.
	RollbackKindAutomatic = "automatic"
	// RolledBackCondition is True on a Revision a completed rollback
	// replaced; Kuvryn Sync does not deploy it again.
	RolledBackCondition = "RolledBack"
)

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
	// suspend stops Kuvryn Sync from mutating managed resources while retaining status.
	// +optional
	Suspend bool `json:"suspend,omitempty"`
	// serviceAccountName is the service account in the Application namespace
	// that Kuvryn Sync impersonates to read, apply, and prune managed resources.
	// When empty, the controller's default service account is used; when
	// neither is set, Kuvryn Sync refuses to touch managed resources.
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`
	// +optional
	ServiceAccountName string `json:"serviceAccountName,omitempty"`
	// notifications sends lifecycle notifications to NotificationSinks in the
	// Application namespace.
	// +listType=atomic
	// +kubebuilder:validation:MaxItems=8
	// +optional
	Notifications []NotificationSubscription `json:"notifications,omitempty"`
	// dependsOn names Applications in the same namespace that must be Healthy
	// at their desired revision before this Application applies. Planning
	// proceeds while they are not.
	// +listType=atomic
	// +kubebuilder:validation:MaxItems=16
	// +optional
	DependsOn []LocalObjectReference `json:"dependsOn,omitempty"`
	// decryption decrypts SOPS-encrypted manifests at render time.
	// +optional
	Decryption *DecryptionSpec `json:"decryption,omitempty"`
}

// DecryptionSpec configures render-time decryption.
type DecryptionSpec struct {
	// provider is the encryption format.
	// +kubebuilder:validation:Enum=sops
	Provider string `json:"provider"`
	// secretRef names a Secret in the Application namespace, labelled
	// sync.kuvryn.io/decryption-key=true, whose entries ending in .agekey hold age
	// private keys.
	SecretRef SecretReference `json:"secretRef"`
}

// NotificationEvent is an Application lifecycle event that can be notified.
// +kubebuilder:validation:Enum=AwaitingApproval;Healthy;Failed;RolledBack
type NotificationEvent string

const (
	// NotificationAwaitingApproval fires when a plan needs manual approval.
	NotificationAwaitingApproval NotificationEvent = "AwaitingApproval"
	// NotificationHealthy fires when a deployment becomes healthy.
	NotificationHealthy NotificationEvent = "Healthy"
	// NotificationFailed fires when a Revision fails.
	NotificationFailed NotificationEvent = "Failed"
	// NotificationRolledBack fires when a rollback completes.
	NotificationRolledBack NotificationEvent = "RolledBack"
)

// NotificationSubscription sends selected events to one NotificationSink.
type NotificationSubscription struct {
	// sinkRef names a NotificationSink in the Application namespace.
	SinkRef LocalObjectReference `json:"sinkRef"`
	// events selects which lifecycle events are sent.
	// +listType=set
	// +kubebuilder:validation:MinItems=1
	Events []NotificationEvent `json:"events"`
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
	// releaseName is the Helm release name used for template rendering. Like
	// Helm, it must be a lowercase DNS subdomain of at most 53 characters.
	// Empty means the default, kuvryn-sync.
	// +kubebuilder:validation:MaxLength=53
	// +kubebuilder:validation:Pattern=`^([a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*)?$`
	// +optional
	ReleaseName string `json:"releaseName,omitempty"`
	// valuesFiles are repository-relative values files.
	// +listType=atomic
	// +optional
	ValuesFiles []string `json:"valuesFiles,omitempty"`
	// chart pulls a pinned chart from a Helm (https) or OCI (oci://)
	// repository instead of rendering source.path as a chart.
	// +optional
	Chart *HelmChartSource `json:"chart,omitempty"`
	// valuesFrom merges values from ConfigMaps and Secrets in the Application
	// namespace, read as the Application's service account, after valuesFiles.
	// +listType=atomic
	// +kubebuilder:validation:MaxItems=16
	// +optional
	ValuesFrom []HelmValuesReference `json:"valuesFrom,omitempty"`
	// values are merged last, over every other source.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	// +kubebuilder:validation:Type=object
	// +optional
	Values *apiextensionsv1.JSON `json:"values,omitempty"`
}

// HelmChartSource identifies a chart in a chart repository.
type HelmChartSource struct {
	// repository is an https chart repository URL or an oci:// registry path.
	// +kubebuilder:validation:Pattern=`^(https|oci)://`
	Repository string `json:"repository"`
	// name is the chart name.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
	// version is the exact chart version to pull.
	// +kubebuilder:validation:MinLength=1
	Version string `json:"version"`
	// secretRef names a Secret, labelled sync.kuvryn.io/registry-credentials=true,
	// with `username` and `password` for the repository.
	// +optional
	SecretRef *SecretReference `json:"secretRef,omitempty"`
}

// HelmValuesReference names values stored in a ConfigMap or Secret.
type HelmValuesReference struct {
	// kind is ConfigMap or Secret.
	// +kubebuilder:validation:Enum=ConfigMap;Secret
	Kind string `json:"kind"`
	// name is the object name in the Application namespace.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
	// key holds a YAML values document; it defaults to values.yaml.
	// +optional
	Key string `json:"key,omitempty"`
}

// ApplicationDestination constrains where desired objects are applied.
type ApplicationDestination struct {
	// namespace is the default namespace for namespaced desired resources.
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// SyncPolicy controls sync behavior.
type SyncPolicy struct {
	// automatic allows Kuvryn Sync to apply approved plans without a separate command.
	// +optional
	Automatic bool `json:"automatic,omitempty"`
	// prune allows deletion of previously managed objects no longer in desired state.
	// +optional
	Prune bool `json:"prune,omitempty"`
	// selfHeal allows Kuvryn Sync to correct managed drift.
	// +optional
	SelfHeal bool `json:"selfHeal,omitempty"`
	// conflictPolicy controls SSA ownership conflict behavior.
	// +kubebuilder:validation:Enum=fail;adopt
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
	// desiredRevision is the source revision Git currently asks Kuvryn Sync to run.
	// +optional
	DesiredRevision string `json:"desiredRevision,omitempty"`
	// deployedRevision is the source revision currently deployed after rollback.
	// +optional
	DeployedRevision string `json:"deployedRevision,omitempty"`
	// serviceAccountName is the service account Kuvryn Sync last impersonated for
	// this Application.
	// +optional
	ServiceAccountName string `json:"serviceAccountName,omitempty"`
	// sync reports desired/live convergence independent from health.
	// +optional
	Sync ApplicationSyncStatus `json:"sync,omitempty"`
	// health reports operational health independent from sync.
	// +optional
	Health ApplicationHealthStatus `json:"health,omitempty"`
	// resources summarizes managed resource health.
	// +optional
	Resources ResourceHealthSummary `json:"resources,omitempty"`
	// diagnosis explains why the Application is not Healthy: each cause names
	// the resource at the root of a failure and the chain of resources from
	// an unhealthy managed object down to it. It is empty when the
	// Application is Healthy.
	// +listType=atomic
	// +kubebuilder:validation:MaxItems=10
	// +optional
	Diagnosis []DiagnosisCause `json:"diagnosis,omitempty"`
	// managedKinds lists the kinds Kuvryn Sync last applied for this Application.
	// Pruning and drift watches use it to find managed objects of any kind,
	// including after a controller restart.
	// +listType=atomic
	// +kubebuilder:validation:MaxItems=256
	// +optional
	ManagedKinds []ManagedKind `json:"managedKinds,omitempty"`
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

// DiagnosisCause is one root cause of an unhealthy Application.
type DiagnosisCause struct {
	// resource is the resource at the root of the failure, such as a missing
	// Secret or a Pod whose container cannot start.
	Resource ResourceRef `json:"resource"`
	// reason is a CamelCase word naming the failure, such as MissingSecret,
	// ImagePullBackOff or Unschedulable.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=128
	// +kubebuilder:validation:Pattern=`^[A-Z][A-Za-z0-9]*$`
	Reason string `json:"reason"`
	// message is the redacted, bounded evidence for the failure.
	// +kubebuilder:validation:MaxLength=512
	// +optional
	Message string `json:"message,omitempty"`
	// chain lists the resources from the unhealthy managed object down to the
	// root cause, both included.
	// +listType=atomic
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=10
	Chain []ResourceRef `json:"chain"`
}

// ManagedKind identifies a kind of object Kuvryn Sync manages for an Application.
type ManagedKind struct {
	// apiVersion is the group/version of the kind.
	APIVersion string `json:"apiVersion"`
	// kind is the object kind.
	Kind string `json:"kind"`
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

// DestinationNamespace is the namespace the Application deploys to: its
// spec.destination.namespace, or its own namespace when that is empty.
func (a *Application) DestinationNamespace() string {
	if a.Spec.Destination.Namespace != "" {
		return a.Spec.Destination.Namespace
	}
	return a.Namespace
}

func init() {
	SchemeBuilder.Register(&Application{}, &ApplicationList{})
}
