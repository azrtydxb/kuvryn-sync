# Prune deletes only eligible managed resources

Status: done
Created: 2026-09-17
Epic: ordering-and-pruning
Sprint: 005-phase-3-ssa-apply-and-sync-policy

## Description

Prune deletes only eligible managed resources.

## Acceptance criteria

- [x] Opt-out annotation prevents prune.
- [x] Previously managed but no-longer desired resources are pruned only when policy allows.
- [x] Application deletion defaults to Orphan.

## Evidence

- Evidence: `internal/prune.Plan` admits only resources labeled for the current Application, honors `solder.io/prune: disabled`, rejects high-risk resources unless policy allows them, and returns reverse apply order for eligible deletes. Tests cover unmanaged, opt-out, and high-risk candidates.

## Correction

- Historical correction: previous evidence was helper/doc oriented; later all-gap closure added controller/runtime, image, installer, CI, KW validation, and product e2e evidence.

## Closure Evidence

- Closed after the all-gap closure pass: product paths now include controller-integrated render/plan/apply/prune/health/drift/retry/rollback handling, bounded metrics and optional OTel tracing, lifecycle Events, CLI operations, installer/CI polish, and regenerated API/manifests. Verification evidence is the final `make fmt test`, `procoder test`, `procoder lint`, `procoder security`, and `procoder check` gate run for this pass.
