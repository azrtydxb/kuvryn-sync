package contracts

import (
	"fmt"
	"sort"

	corev1alpha1 "github.com/azrtydxb/solder/api/v1alpha1"
)

const (
	LineageRepositoryAnnotation = "solder.io/source-repository"
	LineageRevisionAnnotation   = "solder.io/source-revision"
	LineageApplicationLabel     = "solder.io/application"
)

// LifecycleEventType is a stable public event reason.
type LifecycleEventType string

const (
	EventPlanned    LifecycleEventType = "SolderPlanned"
	EventApproved   LifecycleEventType = "SolderApproved"
	EventApplied    LifecycleEventType = "SolderApplied"
	EventHealthy    LifecycleEventType = "SolderHealthy"
	EventFailed     LifecycleEventType = "SolderFailed"
	EventRolledBack LifecycleEventType = "SolderRolledBack"
)

// ValidateProvenance ensures Revision provenance stays provider-neutral.
func ValidateProvenance(prov *corev1alpha1.Provenance) error {
	if prov == nil {
		return nil
	}
	if prov.Source != nil && prov.Source.Repository == "" && prov.Source.Revision == "" {
		return fmt.Errorf("source provenance must identify repository or revision")
	}
	if prov.Artifact != nil && prov.Artifact.URI == "" && prov.Artifact.Digest == "" {
		return fmt.Errorf("artifact provenance must identify URI or digest")
	}
	if prov.Pipeline != nil && prov.Pipeline.Provider == "" && prov.Pipeline.Run == "" {
		return fmt.Errorf("pipeline provenance must identify provider or run")
	}
	return nil
}

// DholeObservation is the public data Dhole can observe without deploying.
type DholeObservation struct {
	Application string                        `json:"application"`
	Revision    string                        `json:"revision"`
	Phase       corev1alpha1.RevisionPhase    `json:"phase"`
	Health      corev1alpha1.HealthState      `json:"health"`
	Failure     *corev1alpha1.RevisionFailure `json:"failure,omitempty"`
}

func ObserveForDhole(app corev1alpha1.Application, rev corev1alpha1.Revision) DholeObservation {
	return DholeObservation{Application: app.Name, Revision: rev.Spec.Source.Revision, Phase: rev.Status.Phase, Health: app.Status.Health.State, Failure: rev.Status.Failure}
}

// KuvrynDiscovery lists public CRDs and actions a UI may use.
type KuvrynDiscovery struct {
	Resources []string `json:"resources"`
	Actions   []string `json:"actions"`
}

func DiscoverForKuvryn() KuvrynDiscovery {
	return KuvrynDiscovery{Resources: []string{"repositories.solder.io", "applications.solder.io", "revisions.solder.io"}, Actions: []string{"plan", "approve", "sync", "suspend", "resume", "rollback"}}
}

// PublicActionPatch describes a Kuvryn-safe public Kubernetes API patch target.
type PublicActionPatch struct {
	Resource string `json:"resource"`
	Name     string `json:"name"`
	Patch    string `json:"patch"`
}

func ValidatePublicAction(action PublicActionPatch) error {
	allowed := map[string]struct{}{"applications.solder.io": {}, "revisions.solder.io": {}}
	if _, ok := allowed[action.Resource]; !ok {
		return fmt.Errorf("action must target public Solder CRDs")
	}
	if action.Name == "" || action.Patch == "" {
		return fmt.Errorf("action name and patch are required")
	}
	return nil
}

func LineageAnnotations(repository, revision string) map[string]string {
	return map[string]string{LineageRepositoryAnnotation: repository, LineageRevisionAnnotation: revision}
}

func StableEventTypes() []LifecycleEventType {
	out := []LifecycleEventType{EventPlanned, EventApproved, EventApplied, EventHealthy, EventFailed, EventRolledBack}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
