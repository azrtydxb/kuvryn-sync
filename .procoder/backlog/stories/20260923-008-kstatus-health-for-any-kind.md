# kstatus health for any kind

Status: open
Created: 2026-09-23
Epic: generic-resource-support
Sprint: -

## Description

As an operator, I see accurate health for resources that follow Kubernetes status conventions without writing any configuration.

## Acceptance criteria

- [ ] Kinds without a built-in evaluator are judged by kstatus rules (`observedGeneration`, `Ready`/`Reconciling`/`Stalled` conditions).
- [ ] Resources with no status at all are Healthy once they exist, instead of `Unknown`.
- [ ] Unit tests cover Current, InProgress, Failed, and status-less inputs.

## Evidence

