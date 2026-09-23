# kstatus health for any kind

Status: done 2026-09-23
Created: 2026-09-23
Epic: generic-resource-support
Sprint: -

## Description

As an operator, I see accurate health for resources that follow Kubernetes status conventions without writing any configuration.

## Acceptance criteria

- [x] Kinds without a built-in evaluator are judged by kstatus rules (`observedGeneration`, `Ready`/`Reconciling`/`Stalled` conditions).
- [x] Resources with no status at all are Healthy once they exist, instead of `Unknown`.
- [x] Unit tests cover Current, InProgress, Failed, and status-less inputs.

## Evidence

- `internal/health/health.go` `generic` applies kstatus rules to every kind without a dedicated rule; Jobs gained Complete/Failed rules.
- `TestEvaluateFollowsKstatusForOtherKinds` covers current, unobserved generation, Reconciling, Ready=False, Stalled, and a status-less object (Healthy); `TestEvaluateJobs` covers Jobs.
- Controller: `completes a status-less custom resource as Healthy without waiting for the health timeout` fails when the old Unknown behaviour is restored (mutation checked).
