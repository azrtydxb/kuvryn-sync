# Revision retention honors history limit

Status: done
Created: 2026-09-17
Epic: bounded-history-and-gc
Sprint: 008-phase-6-history-rollback-retry

## Description

Revision retention honors history limit.

## Acceptance criteria

- [x] Old Revisions are garbage-collected according to Application history policy.
- [x] GC preserves bounded summaries needed for audit.

## Evidence

- Evidence: `internal/history.ApplyRetention` deterministically keeps newest Revisions according to Application history limits and returns older audit records for GC. Tests verify newest retention and limit defaulting.

## Correction

- Historical correction: previous evidence was helper/doc oriented; later all-gap closure added controller/runtime, image, installer, CI, KW validation, and product e2e evidence.

## Closure Evidence

- Closed after the all-gap closure pass: product paths now include controller-integrated render/plan/apply/prune/health/drift/retry/rollback handling, bounded metrics and optional OTel tracing, lifecycle Events, CLI operations, installer/CI polish, and regenerated API/manifests. Verification evidence is the final `make fmt test`, `procoder test`, `procoder lint`, `procoder security`, and `procoder check` gate run for this pass.
