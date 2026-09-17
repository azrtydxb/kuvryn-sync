# Destructive changes are conspicuous

Status: done
Created: 2026-09-17
Epic: secret-redaction-and-plan-safety
Sprint: 004-phase-2-plan-safety-and-cli

## Description

Destructive changes are conspicuous.

## Acceptance criteria

- [x] Deletes are visually and structurally obvious.
- [x] High-risk prune candidates require safety policy checks.

## Evidence

- Evidence: planner marks deletes with `destructive: true`, carries warnings for destructive deletes, prune opt-out annotations, and high-risk resource kinds, and text CLI renders destructive deletes as `! DELETE`. Tests cover destructive and high-risk prune metadata.

## Correction

- Historical correction: previous evidence was helper/doc oriented; later all-gap closure added controller/runtime, image, installer, CI, KW validation, and product e2e evidence.

## Closure Evidence

- Closed after the all-gap closure pass: product paths now include controller-integrated render/plan/apply/prune/health/drift/retry/rollback handling, bounded metrics and optional OTel tracing, lifecycle Events, CLI operations, installer/CI polish, and regenerated API/manifests. Verification evidence is the final `make fmt test`, `procoder test`, `procoder lint`, `procoder security`, and `procoder check` gate run for this pass.
