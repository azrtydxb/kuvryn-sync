# Token 3: Sign in with a token and keep it in the session cookie

Status: open
Created: 2026-09-25
Plan: .procoder/plans/console-token-signin.md

## Description

Add `POST /auth/token`: normalise the token, validate it with a
SelfSubjectReview sent with that token through a client that may make only
that call (10 s timeout), refuse anonymous and unauthenticated identities,
and seal the token into the session cookie as `m`/`t` with an expiry of
min(JWT exp, 8h). The token must never reach a log, a response or a URL
(spec S-1, S-4, S-5).

## Acceptance criteria

- [ ] `TestTokenSignIn` (envtest) failed before the change and passes after it
- [ ] `TestTokenNeverLeaves` failed before the change and passes after it
- [ ] A real ServiceAccount token's sealed cookie is under 4000 bytes, and a larger one is refused (`TestTokenSessionCookieFitsInABrowser`)
- [ ] A cross-site POST to `/auth/token` is refused
- [ ] A 0.4.x cookie without `m` still signs in as an OIDC session

## Evidence
