# Phase 4 — Health + Graph

Status: done
Created: 2026-09-17
Spec: solder-full-product

## Goal

Solder explains rollout success/failure through graph-backed health and diagnosis.

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

## Superseded by the 0.2.0 cleanup

The `internal/graph`, `internal/diagnosis`, and `internal/drift` packages were removed because nothing used them; see the notes on stories 032, 033, and 036–040 for where each behaviour lives now or that it never shipped.
