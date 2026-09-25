# Console 4: Serve the read-only JSON API with RBAC-following namespaces

Status: open
Created: 2026-09-25
Plan: .procoder/plans/kuvryn-sync-console.md

## Description

API handlers, view models, the namespace fallback and timeouts.

## Acceptance criteria

- [ ] `TestConsoleFollowsUserRBAC` and `TestConsoleNeverReturnsSecrets` fail first, then pass
- [ ] `procoder check` has no blocking findings for the resulting change.

## Evidence
