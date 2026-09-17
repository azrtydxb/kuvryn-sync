package diagnosis

import (
	"sort"

	corev1alpha1 "github.com/azrtydxb/solder/api/v1alpha1"
	"github.com/azrtydxb/solder/internal/graph"
	"github.com/azrtydxb/solder/internal/health"
	"github.com/azrtydxb/solder/internal/resource"
)

// Cause is one structured causal diagnosis node.
type Cause struct {
	Resource  resource.ID              `json:"resource"`
	State     corev1alpha1.HealthState `json:"state"`
	Reason    string                   `json:"reason,omitempty"`
	Message   string                   `json:"message,omitempty"`
	DependsOn []resource.ID            `json:"dependsOn,omitempty"`
	Symptoms  []resource.ID            `json:"symptoms,omitempty"`
}

// Build emits deterministic root-cause and symptom chains from graph and health results.
func Build(g graph.Graph, results []health.Result) []Cause {
	byID := map[resource.ID]health.Result{}
	for _, result := range results {
		byID[result.Resource] = result
	}
	causes := []Cause{}
	for _, result := range results {
		if result.State != corev1alpha1.HealthStateDegraded && result.State != corev1alpha1.HealthStateProgressing {
			continue
		}
		cause := Cause{Resource: result.Resource, State: result.State, Reason: result.Reason, Message: result.Message}
		for _, edge := range g.Edges {
			if edge.From == result.Resource {
				if dep, ok := byID[edge.To]; ok && dep.State != corev1alpha1.HealthStateHealthy {
					cause.Symptoms = append(cause.Symptoms, edge.To)
				}
			}
			if edge.To == result.Resource {
				if dep, ok := byID[edge.From]; ok && dep.State != corev1alpha1.HealthStateHealthy {
					cause.DependsOn = append(cause.DependsOn, edge.From)
				}
			}
		}
		sortIDs(cause.DependsOn)
		sortIDs(cause.Symptoms)
		causes = append(causes, cause)
	}
	sort.Slice(causes, func(i, j int) bool { return causes[i].Resource.String() < causes[j].Resource.String() })
	return causes
}

func sortIDs(ids []resource.ID) {
	sort.Slice(ids, func(i, j int) bool { return ids[i].String() < ids[j].String() })
}
