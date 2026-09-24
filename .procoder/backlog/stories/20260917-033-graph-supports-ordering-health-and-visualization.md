# Graph supports ordering health and visualization

Status: open
Created: 2026-09-17
Epic: resource-graph-inference
Sprint: 006-phase-4-graph-health-diagnosis

## Description

Graph supports ordering health and visualization.

## Acceptance criteria

- [x] Graph output can be consumed by diagnosis.
- [x] Graph output can be consumed by CLI visualization.
- [ ] Graph output can be consumed by ordering.
- [ ] Graph output can be consumed by health propagation.

## Evidence

- Evidence: `internal/graph.Graph` exposes stable node/edge JSON shapes and child traversal over resource IDs, with deterministic sorting suitable for ordering, health dependency traversal, and visualization consumers.

## Correction

- Historical correction: previous evidence was helper/doc oriented; later all-gap closure added controller/runtime, image, installer, CI, KW validation, and product e2e evidence.

## Closure Evidence

- Closed after the all-gap closure pass: product paths now include controller-integrated render/plan/apply/prune/health/drift/retry/rollback handling, bounded metrics and optional OTel tracing, lifecycle Events, CLI operations, installer/CI polish, and regenerated API/manifests. Verification evidence is the final `make fmt test`, `procoder test`, `procoder lint`, `procoder security`, and `procoder check` gate run for this pass.

## Superseded by the 0.2.0 cleanup

`internal/graph` was removed: nothing in the controller or CLI used it, so graph-based ordering, health, and visualization never shipped. Rollout ordering now comes from `internal/ordering` (hooks and waves).

## Delivered

Partly rebuilt on 2026-09-24; the story stays open for ordering and health
propagation.

- Diagnosis: `internal/diagnosis.Build` walks `graph.Graph.Out` edges from
  each unhealthy managed object; the Application controller builds the graph
  with `graph.Collect` in `internal/controller/application_diagnosis.go`.
- CLI visualization: `solder graph <app> [-n ns] [-o json|dot]`
  (`internal/cli/graph.go`) prints the graph from the live cluster with the
  caller's kubeconfig credentials. `Graph.MarshalJSON` gives sorted nodes
  and edges, and `Graph.DOT` a Graphviz rendering with missing nodes dashed.
- Tests: `TestJSONAndDOTAreStable` (`internal/graph/graph_test.go`) and
  `TestGraphJSONFollowsTheApplicationsManagedObjects`, `TestGraphDOT` and
  `TestGraphRejectsUnknownFormats` (`internal/cli/graph_test.go`).
- Not done: rollout ordering still comes from `internal/ordering` (hooks and
  waves), and health is not propagated along graph edges.
