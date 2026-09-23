# Phase 4 graph health diagnosis

Status: closed 2026-09-17
Created: 2026-09-17
Milestones: phase-4-health-graph
Spec: solder-full-product

## Goal

Build resource graph and health primitives that can support dependency-aware ordering, rollout observation, visualization, and deterministic diagnosis.

## Committed stories

- [x] `20260917-032-graph-captures-common-built-in-relationships.md` — graph relationships
- [x] `20260917-033-graph-supports-ordering-health-and-visualization.md` — graph output
- [x] `20260917-034-built-in-health-evaluators-cover-mvp-resources.md` — MVP health evaluators
- [x] `20260917-035-rollout-observation-respects-timeout-and-context.md` — rollout observation
- [x] `20260917-036-diagnosis-emits-structured-causal-chains.md` — structured diagnosis

## Execution plan

- [x] Infer common owner, selector, Service, workload, and PVC relationships.
- [x] Provide deterministic graph summaries suitable for ordering, health, and visualization.
- [x] Implement built-in health evaluators for MVP resource kinds.
- [x] Implement context/timeout-aware rollout observation over evaluator snapshots.
- [x] Emit structured causal diagnosis chains.
- [x] Run gates.

## Sprint acceptance

- [x] Graph inference is deterministic and tested.
- [x] Health evaluators cover core workload/service/config resource kinds.
- [x] Rollout observation exits on context cancellation, timeout, healthy, or degraded.
- [x] Diagnosis output names root causes and dependent symptoms.
- [x] `procoder test` and `procoder check` pass with no blockers.

## Retro

Graph and health stayed testable by operating on unstructured snapshots instead of controller state.

Next sprint should use these primitives from watches/drift loops and add real owner labels/watches without changing the evaluator contracts.

Keep structured outputs stable and deterministic before adding richer resource-specific heuristics.

## Result

committed: 5
done: 5 (20260917-032-graph-captures-common-built-in-relationships, 20260917-033-graph-supports-ordering-health-and-visualization, 20260917-034-built-in-health-evaluators-cover-mvp-resources, 20260917-035-rollout-observation-respects-timeout-and-context, 20260917-036-diagnosis-emits-structured-causal-chains)
carried: 0

## Retro

<!-- What slowed us down this sprint. -->

<!-- What we change next sprint because of it. -->

<!-- One adaptation from this sprint worth keeping. -->

## Superseded by the 0.2.0 cleanup

The `internal/graph`, `internal/diagnosis`, and `internal/drift` packages were removed because nothing used them; see the notes on stories 032, 033, and 036–040 for where each behaviour lives now or that it never shipped.
