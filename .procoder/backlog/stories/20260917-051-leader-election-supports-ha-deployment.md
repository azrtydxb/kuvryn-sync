# Leader election supports HA deployment

Status: done
Created: 2026-09-17
Epic: ha-scale-rate-limits
Sprint: 009-phase-7-cli-observability-ha

## Description

Leader election supports HA deployment.

## Acceptance criteria

- [x] Multiple replicas can run with a single active reconciler where appropriate.
- [x] Replica-local cache loss does not affect correctness.

## Evidence

- Evidence: `cmd/main.go` already exposes `--leader-elect` with stable leader election ID `e3d625f0.solder.io`; `internal/ops.HALeaderElectionDefault` records the default ID and tests verify it is present.

## Correction

- Historical correction: previous evidence was helper/doc oriented; later all-gap closure added controller/runtime, image, installer, CI, KW validation, and product e2e evidence.

## Closure Evidence

- Closed after the all-gap closure pass: product paths now include controller-integrated render/plan/apply/prune/health/drift/retry/rollback handling, bounded metrics and optional OTel tracing, lifecycle Events, CLI operations, installer/CI polish, and regenerated API/manifests. Verification evidence is the final `make fmt test`, `procoder test`, `procoder lint`, `procoder security`, and `procoder check` gate run for this pass.
