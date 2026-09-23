# Unknown health no longer hangs rollouts

Status: open
Created: 2026-09-23
Epic: generic-resource-support
Sprint: -

## Description

As an operator, I never see a deployment wait for its full timeout and roll back only because Solder had no evaluator for a kind.

## Acceptance criteria

- [ ] Only genuinely in-progress resources keep a Revision in Observing; `Unknown` is reported in the resource summary but never blocks completion or triggers a timeout rollback (decided 2026-09-23).
- [ ] A controller test with a status-less custom resource completes Healthy without hitting `health.timeout`.

## Evidence

