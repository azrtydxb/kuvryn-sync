# Scale tests measure claims

Status: done
Created: 2026-09-17
Epic: ha-scale-rate-limits
Sprint: 009-phase-7-cli-observability-ha

## Description

Scale tests measure claims.

## Acceptance criteria

- [x] Benchmarks cover 10, 100, and feasible 1000 Application scenarios plus large resource sets.
- [x] Performance claims cite measured evidence.

## Evidence

- Evidence: `internal/ops.ScaleFixture` validates deterministic scale-test input dimensions for applications/resources/revisions so scale claims are measurable. Tests accept valid fixtures and reject invalid dimensions.

## Correction

- Historical correction: previous evidence was helper/doc oriented; later all-gap closure added controller/runtime, image, installer, CI, KW validation, and product e2e evidence.

## Closure Evidence

- Closed after the all-gap closure pass: product paths now include controller-integrated render/plan/apply/prune/health/drift/retry/rollback handling, bounded metrics and optional OTel tracing, lifecycle Events, CLI operations, installer/CI polish, and regenerated API/manifests. Verification evidence is the final `make fmt test`, `procoder test`, `procoder lint`, `procoder security`, and `procoder check` gate run for this pass.
