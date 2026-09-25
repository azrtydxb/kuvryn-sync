# Console 2: Sign users in with OIDC and an encrypted session

Status: closed 2026-09-25
Created: 2026-09-25
Plan: .procoder/plans/kuvryn-sync-console.md

## Description

PKCE, state and nonce, ID token verification, claim mapping, the AES-GCM session cookie, and connectors.

## Acceptance criteria

- [x] `TestOIDCLoginFlow` fails first, then passes
- [x] `go test ./internal/console/` passes
- [x] `procoder check` has no blocking findings for the resulting change.

## Evidence

- `go test ./internal/console/ -run TestOIDCLoginFlow` before: FAIL to compile, `auth_test.go:11:9: undefined: newTestIssuer`; after: `--- PASS: TestOIDCLoginFlow`.
- Mutation check: disabling the nonce comparison failed the test with `wrong nonce accepted: 303`, and disabling the state comparison with `wrong state accepted: 303`; both restored.
- `go test ./internal/console/`: `ok`, including `TestOIDCSessionIdentityAndRefusals` (system: user and group 403, missing claim 400 naming `email`, unverified email 403, foreign key, expired token and wrong audience 400, 500 groups 400 "too many groups for a session", key rotation and expiry log out, prefixes, unknown connector, logout), `TestServerReportsOIDCReadinessAndRoutesSignIn` (`/healthz` `"oidc":"ready"`, `/api/me` 401 then the user, GET `/logout` never clears the session) and `TestDiscoveryRetriesUntilTheIssuerAnswers`.
- `make test` exit 0; `make lint`: "0 issues."; `make manifests generate` left no diff.
- `procoder check` first blocked on `filepath-clean-misuse` at the SPA's `path.Clean`; replaced with `fs.ValidPath` and added `TestSPARefusesPathTraversal`, then `procoder check`: 0 blocking.
