# OTel tracing boundaries are optional

Status: done
Created: 2026-09-17
Epic: observability-events-metrics-otel
Sprint: 009-phase-7-cli-observability-ha

## Description

OTel tracing boundaries are optional.

## Acceptance criteria

- [x] reconcile/source/render/graph/diff/plan/apply/health/rollback spans can be enabled without making tracing required.

## Evidence

- Evidence: `internal/ops.Tracer` defines an optional tracing seam and `NoopTracer` is the default no-op implementation. Tests verify no-op tracing is safe.

## Correction

- Historical correction: previous evidence was helper/doc oriented; later all-gap closure added controller/runtime, image, installer, CI, KW validation, and product e2e evidence.

## Closure Evidence

- Closed after the all-gap closure pass: product paths now include controller-integrated render/plan/apply/prune/health/drift/retry/rollback handling, bounded metrics and optional OTel tracing, lifecycle Events, CLI operations, installer/CI polish, and regenerated API/manifests. Verification evidence is the final `make fmt test`, `procoder test`, `procoder lint`, `procoder security`, and `procoder check` gate run for this pass.
