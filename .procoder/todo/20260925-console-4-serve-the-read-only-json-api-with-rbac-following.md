# Console 4: Serve the read-only JSON API with RBAC-following namespaces

Status: closed 2026-09-25
Created: 2026-09-25
Plan: .procoder/plans/kuvryn-sync-console.md

## Description

API handlers, view models, the namespace fallback and timeouts.

## Acceptance criteria

- [x] `TestConsoleFollowsUserRBAC` and `TestConsoleNeverReturnsSecrets` fail first, then pass
- [x] `procoder check` has no blocking findings for the resulting change.

## Evidence

- `go test ./internal/console/ -run 'TestConsoleFollowsUserRBAC|TestConsoleNeverReturnsSecrets'` before: FAIL, `api_test.go:213: cluster-wide list for a namespaced user = 404 {"error":"not found"}` (the helpers are test code, so it compiled; the plan's expected message is updated); after: `ok`.
- The envtest API server runs with `authorization-mode=RBAC`; alice, bound in `a` only, gets 403 `"needNamespace":true` cluster-wide, 200 with `web` in `a`, and 403 without `db` in `b`.
- `TestConsoleAPIShowsWhatTheUserMayRead` pins each endpoint's content (diagnosis chain, redacted credential in a message, plan digest, Secret change redacted, ConfigMap row not visible for alice, Secret row `web-tls` from the plan) and, for bob, who may list ConfigMaps and Secrets, a visible ConfigMap row and no `creds`. `TestConsoleRefusesSystemIdentitiesAndAnonymous`, `TestConsoleAPITimesOut` (504 `{"error":"timeout"}`) and `TestViewsShowUnknownForMissingStatus` also pass.
- Mutation check: removing the Secret `Exclude` turned Resources into 403s and failed the content test; reading as the console instead of the user failed `TestConsoleFollowsUserRBAC` with `cluster-wide list for a namespaced user = 200`.
- `TestListManagedNeverListsExcludedKinds` covers the new `applier.ListOptions.Exclude`.
- `make test` exit 0; `make lint`: "0 issues."; `make manifests generate` left no diff.
- `procoder check`: 0 blocking.
