# renderers-normalization-validation

Status: done
Created: 2026-09-17
Milestone: phase-1-source-render
Spec: solder-full-product

## Description

Render YAML/Kustomize/Helm into normalized objects and reject invalid desired state before planning.

## Evidence

- Reopened: one or more child stories still require product-integration evidence; helper-only evidence is not sufficient for closure.

## Correction

- Reopened because at least one child story is open for product integration acceptance.

## Closure Evidence

- Closed after the all-gap closure pass: product paths now include controller-integrated render/plan/apply/prune/health/drift/retry/rollback handling, bounded metrics and optional OTel tracing, lifecycle Events, CLI operations, installer/CI polish, and regenerated API/manifests. Verification evidence is the final `make fmt test`, `procoder test`, `procoder lint`, `procoder security`, and `procoder check` gate run for this pass.
