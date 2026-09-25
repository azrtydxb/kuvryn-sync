# Rebrand 4: Rename the field manager, metrics, and runtime names

Status: closed 2026-09-25
Created: 2026-09-25
Plan: .procoder/plans/kuvryn-sync-rebrand.md

## Description

The field manager becomes kuvryn-sync, metrics move to kuvryn_sync_*, and the cache, leader-election, tracer and service names follow.

## Acceptance criteria

- [x] `TestApplyUsesTheKuvrynSyncFieldManager` and `TestMetricNamesUseKuvrynSyncPrefix` fail first, then pass
- [x] `make test` passes
- [x] `procoder check` has no blocking findings for the resulting change.

## Evidence

- `go test ./internal/applier/ ./internal/ops/` before: FAIL, `FieldManager = "solder", want kuvryn-sync` and `metric solder_application_reconcile_total keeps the old prefix` (plus the duration and lifecycle metrics); after: both packages `ok`.
- `git grep -n 'Name: *"solder_'` outside `.procoder/`: no output.
- `make test` exit 0 (CRD default author regenerated as `Kuvryn Sync` / `kuvryn-sync@localhost`); `make lint`: "0 issues.".
- `procoder check`: 0 blocking.
