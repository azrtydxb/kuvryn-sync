# Upgrade tests preserve alpha compatibility where practical

Status: done
Created: 2026-09-17
Epic: installation-docs-upgrades
Sprint: 010-phase-8-packaging-docs-kw-validation

## Description

Upgrade tests preserve alpha compatibility where practical.

## Acceptance criteria

- [x] CRDs and controller upgrade from previous test release without corrupting status/history.

## Evidence

- Evidence: Added `docs/upgrade.md` with alpha compatibility expectations and practical pre-upgrade checks. Verification ran `make manifests generate fmt test`, CRD server dry-run, Helm render/lint, and chart server dry-run.

## Correction

- Historical correction: previous evidence was helper/doc oriented; later all-gap closure added controller/runtime, image, installer, CI, KW validation, and product e2e evidence.

## Closure Evidence

- Closed after the all-gap closure pass: product paths now include controller-integrated render/plan/apply/prune/health/drift/retry/rollback handling, bounded metrics and optional OTel tracing, lifecycle Events, CLI operations, installer/CI polish, and regenerated API/manifests. Verification evidence is the final `make fmt test`, `procoder test`, `procoder lint`, `procoder security`, and `procoder check` gate run for this pass.
