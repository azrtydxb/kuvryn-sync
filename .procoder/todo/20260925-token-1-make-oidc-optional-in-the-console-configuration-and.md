# Token 1: Make OIDC optional in the console configuration and server

Status: closed 2026-09-25
Created: 2026-09-25
Plan: .procoder/plans/console-token-signin.md

## Description

The console refuses to start without an OIDC issuer, so it cannot run on a
cluster with no identity provider. Make `--oidc-issuer-url` and
`--oidc-client-id` optional but paired, answer 404 on the OIDC routes when
they are unset, list no connectors, and skip the impersonation self-check.
Done when the console starts with no OIDC flags at all (spec S-3).

## Acceptance criteria

- [x] `TestConsoleStartsWithoutOIDC` failed before the change and passes after it
- [x] Setting only one of the two OIDC flags is refused at start with a message
- [x] Without OIDC, `/auth/start` and `/auth/callback` answer 404, `/api/me` lists no connectors and reports `tokenSignIn: true`, and `/healthz` answers 200
- [x] The self-check does not check impersonation when OIDC is off
- [x] Existing OIDC tests still pass

## Evidence

- Red: `go test ./internal/console -run TestConsoleStartsWithoutOIDC` failed to build before the change (`cfg.Validate undefined`, `too many arguments in call to NewAuth`).
- Green: `TestConsoleStartsWithoutOIDC` and `TestInsecureCookiesWithoutOIDCOnlyOnLoopback` PASS in commit 2ad337e. The test checks 404 on /auth/start and /auth/callback, `"connectors":[]`, `"tokenSignIn":true`, /healthz 200 with `"oidc":"disabled"` and `"impersonation":"disabled"`, that the self-check sends no request, and that a lone issuer or client ID is refused by both Validate and NewAuth.
- `make test` exit 0: every OIDC test in internal/console still passes (84.3% coverage).
