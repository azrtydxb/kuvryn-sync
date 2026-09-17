# CLI plan prints operator-friendly diff

Status: done
Created: 2026-09-17
Epic: change-plan-model-and-cli
Sprint: 004-phase-2-plan-safety-and-cli

## Description

CLI plan prints operator-friendly diff.

## Acceptance criteria

- [x] `solder plan` shows changed fields, creates, deletes, unchanged counts, and source revision.
- [x] `-o json` and `-o yaml` expose machine-readable plan data.

## Evidence

- Evidence: `cmd/main.go` now dispatches `solder plan`; `internal/cli` loads the latest cluster Revision or a Revision file fixture and prints text/json/yaml plan output. `internal/planoutput` renders source revision, create/update/delete/unchanged counts, resource actions, field changes, conflicts, and warnings. `internal/cli/plan_test.go` and `internal/planoutput/output_test.go` cover text and JSON output.

## Correction

- Reopened: previous evidence proves library/helper/doc coverage only, not end-to-end product integration through Application reconciliation, Revision lifecycle, CLI, image/runtime, watches, GC/finalizers, real e2e, installer, CI, and docs completion.

## Closure Evidence

- Closed after the all-gap closure pass: product paths now include controller-integrated render/plan/apply/prune/health/drift/retry/rollback handling, bounded metrics and optional OTel tracing, lifecycle Events, CLI operations, installer/CI polish, and regenerated API/manifests. Verification evidence is the final `make fmt test`, `procoder test`, `procoder lint`, `procoder security`, and `procoder check` gate run for this pass.
