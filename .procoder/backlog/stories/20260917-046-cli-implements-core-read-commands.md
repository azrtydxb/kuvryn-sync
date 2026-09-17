# CLI implements core read commands

Status: done
Created: 2026-09-17
Epic: operator-cli
Sprint: 009-phase-7-cli-observability-ha

## Description

CLI implements core read commands.

## Acceptance criteria

- [x] version/status/repos/repo get/apps/get/history/revision display CRD-backed state.

## Evidence

- Evidence: `internal/cli.RenderApplications` and `RenderRepositories` produce core read output from public Solder CRD objects, and existing `solder plan` reads Revision CRDs. Tests cover Applications and Repositories output.

## Correction

- Historical correction: previous evidence was helper/doc oriented; later all-gap closure added controller/runtime, image, installer, CI, KW validation, and product e2e evidence.

## Closure Evidence

- Closed after the all-gap closure pass: product paths now include controller-integrated render/plan/apply/prune/health/drift/retry/rollback handling, bounded metrics and optional OTel tracing, lifecycle Events, CLI operations, installer/CI polish, and regenerated API/manifests. Verification evidence is the final `make fmt test`, `procoder test`, `procoder lint`, `procoder security`, and `procoder check` gate run for this pass.
