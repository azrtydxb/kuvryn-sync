# Failed desired revisions do not loop forever

Status: done
Created: 2026-09-17
Epic: failed-revision-retry-protection
Sprint: 008-phase-6-history-rollback-retry

## Description

Failed desired revisions do not loop forever.

## Acceptance criteria

- [x] maxAttempts/backoff/suspension prevents deploy-fail-rollback loops.
- [x] Desired and deployed revisions are reported honestly after rollback.

## Evidence

- Evidence: `internal/retry.Decide` enforces maxAttempts, backoff windows, and suspension, while preserving honest desired/deployed revision reporting through `ReportHonestRevisions`. Tests cover max-attempt blocking, backoff, suspension, and status reporting.

## Correction

- Reopened: previous evidence proves library/helper/doc coverage only, not end-to-end product integration through Application reconciliation, Revision lifecycle, CLI, image/runtime, watches, GC/finalizers, real e2e, installer, CI, and docs completion.

## Closure Evidence

- Closed after the all-gap closure pass: product paths now include controller-integrated render/plan/apply/prune/health/drift/retry/rollback handling, bounded metrics and optional OTel tracing, lifecycle Events, CLI operations, installer/CI polish, and regenerated API/manifests. Verification evidence is the final `make fmt test`, `procoder test`, `procoder lint`, `procoder security`, and `procoder check` gate run for this pass.
