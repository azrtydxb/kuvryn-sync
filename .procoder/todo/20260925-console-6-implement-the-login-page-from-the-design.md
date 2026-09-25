# Console 6: Implement the login page from the design

Status: closed 2026-09-25
Created: 2026-09-25
Plan: .procoder/plans/kuvryn-sync-console.md

## Description

The Login page in split and centered layouts, dark and light, with connectors and copy from the design.

## Acceptance criteria

- [x] `TestLoginPage` (make test-ui) fails first, then passes in all four cases, with no serious axe violations
- [x] `procoder check` has no blocking findings for the resulting change.

## Evidence

- `make test-ui` before: 4 failed, each `expect(locator).toBeVisible() failed, Locator: getByRole('button', { name: /Sign in with/ }), Error: element(s) not found`; after: `4 passed` (dark and light at 1400px and 700px), with no serious or critical axe violations, emblem `naturalWidth` >= 512, and no console errors (CSP).
- Screenshots of dark, light and narrow-with-error checked against "Kuvryn Sync Login.dc.html"; the emblems are the untracked placeholders.
- `go test ./internal/console/ ./internal/cli/`: `ok` (unauthenticated `/api/me` answers 200 `"authenticated":false` with the cluster, connectors and `ssoName`; `system:` identities get the same; `--sso-name` is listed in `ksync console --help`).
- `npm --prefix web run typecheck` clean, `npm --prefix web test` 1 passed; `make test` exit 0; `make lint`: "0 issues."; `make manifests generate` left no diff.
- `procoder check`: 0 blocking.
