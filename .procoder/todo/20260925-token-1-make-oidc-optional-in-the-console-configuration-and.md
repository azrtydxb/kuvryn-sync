# Token 1: Make OIDC optional in the console configuration and server

Status: open
Created: 2026-09-25
Plan: .procoder/plans/console-token-signin.md

## Description

The console refuses to start without an OIDC issuer, so it cannot run on a
cluster with no identity provider. Make `--oidc-issuer-url` and
`--oidc-client-id` optional but paired, answer 404 on the OIDC routes when
they are unset, list no connectors, and skip the impersonation self-check.
Done when the console starts with no OIDC flags at all (spec S-3).

## Acceptance criteria

- [ ] `TestConsoleStartsWithoutOIDC` failed before the change and passes after it
- [ ] Setting only one of the two OIDC flags is refused at start with a message
- [ ] Without OIDC, `/auth/start` and `/auth/callback` answer 404, `/api/me` lists no connectors and reports `tokenSignIn: true`, and `/healthz` answers 200
- [ ] The self-check does not check impersonation when OIDC is off
- [ ] Existing OIDC tests still pass

## Evidence
