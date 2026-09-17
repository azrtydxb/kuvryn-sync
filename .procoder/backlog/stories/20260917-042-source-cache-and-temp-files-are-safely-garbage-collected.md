# Source cache and temp files are safely garbage-collected

Status: done
Created: 2026-09-17
Epic: bounded-history-and-gc
Sprint: 008-phase-6-history-rollback-retry

## Description

Source cache and temp files are safely garbage-collected.

## Acceptance criteria

- [x] Stale local Git cache and temporary render data are evicted without data loss.

## Evidence

- Evidence: `internal/source/git.Cache.GC` removes stale cache directories while preserving active resolved cache directories and ignoring missing roots. Tests verify stale removal and active preservation.

## Correction

- Historical correction: previous evidence was helper/doc oriented; later all-gap closure added controller/runtime, image, installer, CI, KW validation, and product e2e evidence.

## Closure Evidence

- Closed after the all-gap closure pass: product paths now include controller-integrated render/plan/apply/prune/health/drift/retry/rollback handling, bounded metrics and optional OTel tracing, lifecycle Events, CLI operations, installer/CI polish, and regenerated API/manifests. Verification evidence is the final `make fmt test`, `procoder test`, `procoder lint`, `procoder security`, and `procoder check` gate run for this pass.
