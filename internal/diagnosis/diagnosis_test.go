package diagnosis

import (
	"testing"

	corev1alpha1 "github.com/azrtydxb/solder/api/v1alpha1"
	"github.com/azrtydxb/solder/internal/graph"
	"github.com/azrtydxb/solder/internal/health"
	"github.com/azrtydxb/solder/internal/resource"
)

func TestBuildEmitsStructuredCausalChains(t *testing.T) {
	service := resource.ID{Group: "", Version: "v1", Kind: "Service", Namespace: "payments", Name: "api"}
	deploy := resource.ID{Group: "apps", Version: "v1", Kind: "Deployment", Namespace: "payments", Name: "api"}
	g := graph.Graph{Nodes: []resource.ID{service, deploy}, Edges: []graph.Edge{{From: service, To: deploy, Type: graph.EdgeSelects}}}
	causes := Build(g, []health.Result{
		{Resource: service, State: corev1alpha1.HealthStateHealthy, Reason: "Ready"},
		{Resource: deploy, State: corev1alpha1.HealthStateProgressing, Reason: "ReplicasUnavailable", Message: "1/3 replicas available"},
	})
	if len(causes) != 1 {
		t.Fatalf("causes = %#v", causes)
	}
	if causes[0].Resource != deploy || causes[0].Reason != "ReplicasUnavailable" || len(causes[0].DependsOn) != 0 {
		t.Fatalf("unexpected cause: %#v", causes[0])
	}
	if causes[0].Message == "" {
		t.Fatalf("missing diagnostic message: %#v", causes[0])
	}
}
