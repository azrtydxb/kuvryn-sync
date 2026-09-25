# Token 4: Give each token its own reader and follow its RBAC

Status: closed 2026-09-25
Created: 2026-09-25
Plan: .procoder/plans/console-token-signin.md

## Description

Key cached readers for token sessions by username, groups and the token's
SHA-256, so two tokens never share a reader, build token readers with the
token client, and clear the session when the API server answers 401 because
the token expired or was revoked (spec S-2, S-5).

## Acceptance criteria

- [x] `TestTokenSessionFollowsRBAC` (envtest) failed before the change and passes after it
- [x] Two tokens for the same identity get separate readers, and the cache key never contains the token
- [x] An API 401 in a token session answers 401 and clears the session cookie

## Evidence

- Red before fbdd5ce:
  - `TestTokenSessionFollowsRBAC`: "the revoked token's session still reads", because a reader was shared by username and groups.
  - `TestReaderCacheSeparatesTokens`: "1 builds … want 3".
- Green: the reader sees `web` in a and gets 403 in b. `nobody` gets 403 in a.
- Revoking the first bound token (by deleting its Secret) returns 401 and a cleared `ksync_session` cookie. The second token of the same ServiceAccount keeps reading.
- The cache key holds sha256(token), never the token.
- Note: reader dispatch for token sessions landed with task 3, so the basic RBAC assertions alone would already pass. The revocation assertions are the part that failed first.
