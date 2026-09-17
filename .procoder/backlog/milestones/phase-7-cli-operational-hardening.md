# Phase 7 — CLI + Operational Hardening

Status: done
Created: 2026-09-17
Spec: solder-full-product

## Goal

Solder is operable through CLI, metrics, events, HA, docs, install artifacts, and upgrade/scale tests.

## Success state

- Scope for this phase is represented by epics, stories, and executable task files.
- Acceptance criteria for stories are testable and evidence-oriented.
- Work can be pulled into sprints without rereading the full product specification.

## Evidence

- Historical correction: child epics/stories previously required product-integration evidence; the closure evidence below records the resolved product path.

## Correction

- Historical correction: this milestone previously required end-to-end controller/CLI/runtime verification beyond pure-function coverage; the closure evidence below records that resolved product path.

## Closure Evidence

- Closed after the all-gap closure pass: product paths now include controller-integrated render/plan/apply/prune/health/drift/retry/rollback handling, bounded metrics and optional OTel tracing, lifecycle Events, CLI operations, installer/CI polish, and regenerated API/manifests. Verification evidence is the final `make fmt test`, `procoder test`, `procoder lint`, `procoder security`, and `procoder check` gate run for this pass.
