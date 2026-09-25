# Token 4: Give each token its own reader and follow its RBAC

Status: open
Created: 2026-09-25
Plan: .procoder/plans/console-token-signin.md

## Description

Key cached readers for token sessions by username, groups and the token's
SHA-256, so two tokens never share a reader, build token readers with the
token client, and clear the session when the API server answers 401 because
the token expired or was revoked (spec S-2, S-5).

## Acceptance criteria

- [ ] `TestTokenSessionFollowsRBAC` (envtest) failed before the change and passes after it
- [ ] Two tokens for the same identity get separate readers, and the cache key never contains the token
- [ ] An API 401 in a token session answers 401 and clears the session cookie

## Evidence
