# Docs explain operation and API

Status: done
Created: 2026-09-17
Epic: installation-docs-upgrades
Sprint: 010-phase-8-packaging-docs-kw-validation

## Description

Docs explain operation and API.

## Acceptance criteria

- [x] README/API docs explain Repository/Application/Revision, CLI use, security model, and failure handling.

## Evidence

- Evidence: Added `docs/install.md`, `docs/operations.md`, and `docs/api.md` covering install paths, KW development validation, public CRDs, CLI behavior, safety defaults, and API invariants.

## Correction

- Historical correction: previous evidence was helper/doc oriented; later all-gap closure added controller/runtime, image, installer, CI, KW validation, and product e2e evidence.

## Closure Evidence

- Closed after the all-gap closure pass: product paths now include controller-integrated render/plan/apply/prune/health/drift/retry/rollback handling, bounded metrics and optional OTel tracing, lifecycle Events, CLI operations, installer/CI polish, and regenerated API/manifests. Verification evidence is the final `make fmt test`, `procoder test`, `procoder lint`, `procoder security`, and `procoder check` gate run for this pass.
