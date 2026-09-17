# SSA applier applies desired objects idempotently

Status: done
Created: 2026-09-17
Epic: ssa-sync-engine
Sprint: 005-phase-3-ssa-apply-and-sync-policy

## Description

SSA applier applies desired objects idempotently.

## Acceptance criteria

- [x] Managed resources use stable field manager `solder`.
- [x] Repeated apply converges without unintended side effects.

## Evidence

- Evidence: `internal/applier.Applier` applies unstructured desired objects with controller-runtime server-side apply, Solder field ownership, fail-conflict default behavior, and managed Application/revision metadata. Tests cover repeated idempotent apply and rejection of unsupported force-style conflict policy.

## Correction

- Reopened: previous evidence proves library/helper/doc coverage only, not end-to-end product integration through Application reconciliation, Revision lifecycle, CLI, image/runtime, watches, GC/finalizers, real e2e, installer, CI, and docs completion.

## Closure Evidence

- Closed after the all-gap closure pass: product paths now include controller-integrated render/plan/apply/prune/health/drift/retry/rollback handling, bounded metrics and optional OTel tracing, lifecycle Events, CLI operations, installer/CI polish, and regenerated API/manifests. Verification evidence is the final `make fmt test`, `procoder test`, `procoder lint`, `procoder security`, and `procoder check` gate run for this pass.
