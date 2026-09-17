# Phase 5 — Drift + Self-Heal

Status: done
Created: 2026-09-17
Spec: solder-full-product

## Goal

Solder detects out-of-band managed changes and optionally self-heals safely.

## Success state

- Scope for this phase is represented by epics, stories, and executable task files.
- Acceptance criteria for stories are testable and evidence-oriented.
- Work can be pulled into sprints without rereading the full product specification.

## Evidence

- Reopened: child epics/stories still require product-integration evidence; helper-only evidence is not sufficient for closure.

## Correction

- Reopened: milestone is not complete as a product milestone until its stories are wired into end-to-end controller/CLI/runtime workflows and verified beyond pure-function coverage.

## Closure Evidence

- Closed after the all-gap closure pass: product paths now include controller-integrated render/plan/apply/prune/health/drift/retry/rollback handling, bounded metrics and optional OTel tracing, lifecycle Events, CLI operations, installer/CI polish, and regenerated API/manifests. Verification evidence is the final `make fmt test`, `procoder test`, `procoder lint`, `procoder security`, and `procoder check` gate run for this pass.
