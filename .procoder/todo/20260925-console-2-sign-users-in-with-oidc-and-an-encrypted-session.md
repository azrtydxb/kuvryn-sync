# Console 2: Sign users in with OIDC and an encrypted session

Status: open
Created: 2026-09-25
Plan: .procoder/plans/kuvryn-sync-console.md

## Description

PKCE, state and nonce, ID token verification, claim mapping, the AES-GCM session cookie, and connectors.

## Acceptance criteria

- [ ] `TestOIDCLoginFlow` fails first, then passes
- [ ] `go test ./internal/console/` passes
- [ ] `procoder check` has no blocking findings for the resulting change.

## Evidence
