# Unknown health no longer hangs rollouts

Status: done 2026-09-23
Created: 2026-09-23
Epic: generic-resource-support
Sprint: -

## Description

As an operator, I never see a deployment wait for its full timeout and roll back only because Solder had no evaluator for a kind.

## Acceptance criteria

- [x] Only genuinely in-progress resources keep a Revision in Observing; `Unknown` is reported in the resource summary but never blocks completion or triggers a timeout rollback (decided 2026-09-23).
- [x] A controller test with a status-less custom resource completes Healthy without hitting `health.timeout`.

## Evidence

- `applyAndObserve` now waits only while `summary.Progressing > 0`; Unknown is still counted in `status.resources`.
- `completes a status-less custom resource as Healthy without waiting for the health timeout` uses a 1ms health timeout and a status-less Widget and completes Healthy; restoring Unknown-blocks-rollout together with the old evaluator makes it fail (mutation checked). Since the kstatus evaluator no longer returns Unknown, this branch is defensive.
