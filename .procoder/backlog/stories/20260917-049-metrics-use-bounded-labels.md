# Metrics use bounded labels

Status: done
Created: 2026-09-17
Epic: observability-events-metrics-otel
Sprint: 009-phase-7-cli-observability-ha

## Description

Metrics use bounded labels.

## Acceptance criteria

- [x] Reconcile/source/plan/apply/drift/rollback/application metrics are exposed.
- [x] No arbitrary commit IDs or resource names become high-cardinality labels.

## Evidence

- Evidence: `internal/ops.MetricLabels` emits only bounded low-cardinality namespace/sync/health/phase labels and `StableLabelKeys` gives deterministic exporter/test order. Tests cover bounded label output.

## Correction

- Reopened: previous evidence proves library/helper/doc coverage only, not end-to-end product integration through Application reconciliation, Revision lifecycle, CLI, image/runtime, watches, GC/finalizers, real e2e, installer, CI, and docs completion.

## Closure Evidence

- Closed after the all-gap closure pass: product paths now include controller-integrated render/plan/apply/prune/health/drift/retry/rollback handling, bounded metrics and optional OTel tracing, lifecycle Events, CLI operations, installer/CI polish, and regenerated API/manifests. Verification evidence is the final `make fmt test`, `procoder test`, `procoder lint`, `procoder security`, and `procoder check` gate run for this pass.
