package contracts

import (
	"testing"

	corev1alpha1 "github.com/azrtydxb/solder/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestValidateProvenanceIsProviderNeutral(t *testing.T) {
	if err := ValidateProvenance(&corev1alpha1.Provenance{Source: &corev1alpha1.SourceProvenance{Repository: "git@example/repo", Revision: "abc"}, Artifact: &corev1alpha1.ArtifactProvenance{URI: "oci://image", Digest: "sha256:abc"}, Pipeline: &corev1alpha1.PipelineProvenance{Provider: "dhole", Run: "42"}}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateProvenance(&corev1alpha1.Provenance{Artifact: &corev1alpha1.ArtifactProvenance{}}); err == nil {
		t.Fatal("empty artifact provenance accepted")
	}
}

func TestStableLifecycleEvents(t *testing.T) {
	events := StableEventTypes()
	if len(events) != 6 || events[0] != EventApplied {
		t.Fatalf("events = %#v", events)
	}
}

func TestDholeObservesDeploymentResultWithoutDeploying(t *testing.T) {
	app := corev1alpha1.Application{ObjectMeta: metav1.ObjectMeta{Name: "payments"}, Status: corev1alpha1.ApplicationStatus{Health: corev1alpha1.ApplicationHealthStatus{State: corev1alpha1.HealthStateHealthy}}}
	rev := corev1alpha1.Revision{Spec: corev1alpha1.RevisionSpec{Source: corev1alpha1.RevisionSource{Revision: "abc123"}}, Status: corev1alpha1.RevisionStatus{Phase: corev1alpha1.RevisionPhaseHealthy}}
	obs := ObserveForDhole(app, rev)
	if obs.Application != "payments" || obs.Revision != "abc123" || obs.Health != corev1alpha1.HealthStateHealthy {
		t.Fatalf("observation = %#v", obs)
	}
}

func TestKuvrynDiscoveryAndActionsUsePublicAPI(t *testing.T) {
	discovery := DiscoverForKuvryn()
	if len(discovery.Resources) != 3 || len(discovery.Actions) == 0 {
		t.Fatalf("discovery = %#v", discovery)
	}
	if err := ValidatePublicAction(PublicActionPatch{Resource: "applications.solder.io", Name: "payments", Patch: "spec:\n  suspend: true\n"}); err != nil {
		t.Fatal(err)
	}
	if err := ValidatePublicAction(PublicActionPatch{Resource: "deployments.apps", Name: "api", Patch: "{}"}); err == nil {
		t.Fatal("private/non-Solder action accepted")
	}
}

func TestLineageAnnotations(t *testing.T) {
	annotations := LineageAnnotations("git@example/repo", "abc123")
	if annotations[LineageRepositoryAnnotation] != "git@example/repo" || annotations[LineageRevisionAnnotation] != "abc123" {
		t.Fatalf("annotations = %#v", annotations)
	}
}
