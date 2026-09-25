# Console 7: Implement the console pages from the design

Status: closed 2026-09-25
Created: 2026-09-25
Plan: .procoder/plans/kuvryn-sync-console.md

## Description

The Shell, Applications, Application detail tabs, Repositories, Revisions, Image policies, the namespace picker and the hack/console-dev seeded server.

## Acceptance criteria

- [x] `TestConsolePages` and `TestLiveRefresh` (make test-ui) fail first, then pass
- [x] `procoder check` has no blocking findings for the resulting change.

## Evidence

- `make test-ui` before: `TestConsolePages` failed with `Locator: locator('.az-stat').filter({ hasText: 'Applications' })`, `element(s) not found`; `TestLiveRefresh` failed with `Locator: getByRole('row', { name: /catalog.*Healthy/ })` not visible.
- After: `6 passed` (`TestConsolePages`, `TestLiveRefresh` and the four `TestLoginPage` cases). TestLiveRefresh's change showed up without a reload within one refresh (the test takes about 11s).
- Screenshots of Applications (dark and light), payments' Diagnosis and Resources, checkout's Plan, Repositories, Revisions and Image policies checked against "Kuvryn Sync Console.dc.html".
- `go test ./internal/console/`: `ok`, including the new `TestNewestFirstPrefersStartTime` (it first failed to compile with `undefined: shortDuration`).
- `npm --prefix web run typecheck` clean; `make test` exit 0; `make lint`: "0 issues."; `make manifests generate` left no diff.
- `procoder check`: 0 blocking.
