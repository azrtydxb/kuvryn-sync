# Rebrand 4: Rename the field manager, metrics, and runtime names

Status: open
Created: 2026-09-25
Plan: .procoder/plans/kuvryn-sync-rebrand.md

## Description

The field manager becomes kuvryn-sync, metrics move to kuvryn_sync_*, and the cache, leader-election, tracer and service names follow.

## Acceptance criteria

- [ ] `TestApplyUsesTheKuvrynSyncFieldManager` and `TestMetricNamesUseKuvrynSyncPrefix` fail first, then pass
- [ ] `make test` passes
- [ ] `procoder check` has no blocking findings for the resulting change.

## Evidence
