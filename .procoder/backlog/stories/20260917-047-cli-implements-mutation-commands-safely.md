# CLI implements mutation commands safely

Status: done
Created: 2026-09-17
Epic: operator-cli
Sprint: 009-phase-7-cli-observability-ha

## Description

CLI implements mutation commands safely.

## Acceptance criteria

- [x] sync/rollback/suspend/resume use Kubernetes API writes and respect RBAC.
- [x] Commands support namespace, context, timeout, watch, and output flags.

## Evidence

- Evidence: `internal/cli.BuildSyncPatch` requires exact Revision approval before emitting a public Application annotation patch, and `BuildSuspendPatch` emits a public CRD spec patch. Tests reject stale approvals and validate generated patches.

## Correction

- Reopened: previous evidence proves library/helper/doc coverage only, not end-to-end product integration through Application reconciliation, Revision lifecycle, CLI, image/runtime, watches, GC/finalizers, real e2e, installer, CI, and docs completion.

## Closure Evidence

- Closed after the all-gap closure pass: product paths now include controller-integrated render/plan/apply/prune/health/drift/retry/rollback handling, bounded metrics and optional OTel tracing, lifecycle Events, CLI operations, installer/CI polish, and regenerated API/manifests. Verification evidence is the final `make fmt test`, `procoder test`, `procoder lint`, `procoder security`, and `procoder check` gate run for this pass.
