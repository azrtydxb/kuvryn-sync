# Token 3: Sign in with a token and keep it in the session cookie

Status: closed 2026-09-25
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

- [x] `TestTokenSignIn` (envtest) failed before the change and passes after it
- [x] `TestTokenNeverLeaves` failed before the change and passes after it
- [x] A real ServiceAccount token's sealed cookie is under 4000 bytes, and a larger one is refused (`TestTokenSessionCookieFitsInABrowser`)
- [x] A cross-site POST to `/auth/token` is refused
- [x] A 0.4.x cookie without `m` still signs in as an OIDC session

## Evidence

- Red before 1002e8b:
  - `TestTokenSignIn`: "valid token = 404".
  - `TestTokenNeverLeaves`: "sign-in failed: 404".
  - `TestTokenSessionCookieFitsInABrowser`: 404.
  - `TestOIDCCookieWithoutMethodStillWorks`: the SA token cookie was refused and the unknown method was accepted.
- Green: all five tests PASS against envtest (Kubernetes 1.35).
  - Refused with error=token: garbage, forged and expired JWTs, 16 KiB + 1, an inner space, anonymous, unauthenticated and node users.
  - Sent to error=cluster: 404, 500 and an unreachable API server.
  - An oversized, expired or control-character token makes no review request.
- Mutation checks on `TestTokenNeverLeaves`: logging the token failed the test, and so did adding 9h to the sealed expiry.
- Cookie size: a 1426-byte token for the longest namespace and ServiceAccount names seals to a 2753-byte cookie (under 4000). A 3500-byte token is refused with error=token.
- Cross-site POSTs (Sec-Fetch-Site, Origin) get 403 and send no review. A 0.4.x cookie without `m` reads as `"method":"oidc"`.
