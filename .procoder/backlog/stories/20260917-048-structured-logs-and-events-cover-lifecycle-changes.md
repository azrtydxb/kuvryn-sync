# Structured logs and Events cover lifecycle changes

Status: done
Created: 2026-09-17
Epic: observability-events-metrics-otel
Sprint: 009-phase-7-cli-observability-ha

## Description

Structured logs and Events cover lifecycle changes.

## Acceptance criteria

- [x] Logs include application, repository, revision, namespace, resource, and reconcile_id.
- [x] Events are concise and meaningful, not per-loop noise.

## Evidence

- Evidence: `internal/ops.NewLifecycleEvent` provides bounded structured lifecycle event/log fields for Application, Revision, phase, reason, and message without secret values. Tests cover event shape.

## Correction

- Historical correction: previous evidence was helper/doc oriented; later all-gap closure added controller/runtime, image, installer, CI, KW validation, and product e2e evidence.

## Closure Evidence

- Closed after the all-gap closure pass: product paths now include controller-integrated render/plan/apply/prune/health/drift/retry/rollback handling, bounded metrics and optional OTel tracing, lifecycle Events, CLI operations, installer/CI polish, and regenerated API/manifests. Verification evidence is the final `make fmt test`, `procoder test`, `procoder lint`, `procoder security`, and `procoder check` gate run for this pass.
