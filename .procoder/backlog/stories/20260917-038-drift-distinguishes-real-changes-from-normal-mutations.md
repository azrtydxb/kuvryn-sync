# Drift distinguishes real changes from normal mutations

Status: done
Created: 2026-09-17
Epic: drift-calculation-and-status
Sprint: 007-phase-5-drift-and-self-heal

## Description

Drift distinguishes real changes from normal mutations.

## Acceptance criteria

- [x] Replica/image/config drift is reported.
- [x] Server-generated and cooperating-controller fields do not cause false drift.

## Evidence

- Evidence: `internal/drift.Classify` reuses the normalizer/planner pipeline so server mutations such as resourceVersion/status are ignored while meaningful spec/data changes become Drifted. Tests cover both cases.

## Correction

- Historical correction: previous evidence was helper/doc oriented; later all-gap closure added controller/runtime, image, installer, CI, KW validation, and product e2e evidence.

## Closure Evidence

- Closed after the all-gap closure pass: product paths now include controller-integrated render/plan/apply/prune/health/drift/retry/rollback handling, bounded metrics and optional OTel tracing, lifecycle Events, CLI operations, installer/CI polish, and regenerated API/manifests. Verification evidence is the final `make fmt test`, `procoder test`, `procoder lint`, `procoder security`, and `procoder check` gate run for this pass.
