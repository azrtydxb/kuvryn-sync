# console-token-signin

Status: complete

## Problem

The read-only console (`ksync console`) can only sign people in through an
external OIDC provider. A plain Kubernetes cluster has none, so the console
cannot be used there at all. kw is such a cluster: it has no Dex or other
identity provider, and exposing the console at `sync.kw.watteel.lab` stalled on
that. Tools people already run on these clusters, such as Headlamp and the
Kubernetes Dashboard, work with nothing but a Kubernetes bearer token. The
console should work the same way out of the box, with OIDC as an optional
extra.

## Users

- **Operators on a vanilla cluster:** want the console running with no identity provider to install first.
- **Application teams and auditors:** sign in with a token an operator gives them, for example from `kubectl create token`, and see exactly what that token's RBAC allows.
- **Clusters that already have SSO:** keep the existing OIDC sign-in unchanged.

## In scope

- [S-1] Token sign-in. The login page always offers a "Kubernetes token" field. `POST /auth/token` (form field `token`) validates the token against the API server with a `SelfSubjectReview` sent with that token, and on success starts a session.
- [S-2] Reads run as the token. In a token session every API request carries the user's bearer token and never the console's own ServiceAccount credentials or any `Impersonate-*` header. The existing read-only rules in `internal/console/kube.go` (`readOnlyTransport`, `forbiddenPath`) still apply: GET only, only the allowed path shapes, no core Secrets, no subresources, no watches.
- [S-3] OIDC becomes optional. `--oidc-issuer-url` and `--oidc-client-id` are no longer required. Without them the OIDC routes answer 404, `/api/me` lists no connectors, and the login page shows only the token form. With them, OIDC sign-in works as before, next to the token form.
- [S-4] Identity rules for token sessions. `system:anonymous` and any identity in the `system:unauthenticated` group are refused. ServiceAccount identities (`system:serviceaccount:...`) are allowed, because the token is used as is and nothing is impersonated. OIDC sessions keep their current rules.
- [S-5] Session handling for tokens. The token is sealed in the existing AES-256-GCM session cookie, never logged, never put in a URL, and never returned by any API. The session expires at the token's JWT `exp` when it can be decoded, and at most 8 hours after sign-in.
- [S-6] Chart. With `console.enabled=true` and no `console.oidc.issuerURL`, the chart renders a console that uses token sign-in only. It renders no impersonate ClusterRole and no ClusterRoleBinding for the console, does not require the redirect URL value, and passes no OIDC flags. The self-check reports impersonation only when OIDC is configured.
- [S-7] Documentation. `docs/console.md` covers:
  - token sign-in first, OIDC as optional;
  - how to mint a read-only viewer token (`kubectl create token`, with a matching ServiceAccount and RoleBinding example);
  - how to add the console to a raw-manifest (`dist/install.yaml`) install with `helm template --show-only templates/console.yaml`;
  - `docs/security.md` states what a token session can and cannot do.

## Out of scope

- Built-in local users or passwords.
- Kubeconfig upload.
- Revoking a token: signing out clears the console session only.
- Rate limiting sign-in attempts.
- Any write action in the console.
- A chart-managed viewer ServiceAccount.
- Installing an identity provider.

## Constraints

- The console must never read as its own ServiceAccount in a token session.
- The existing CSP, cookie flags (HttpOnly, Secure unless `--insecure-cookies`, SameSite=Lax) and cross-origin protection apply to `POST /auth/token`.
- Tokens larger than 16 KiB are refused before any request is made.
- `SelfSubjectReview` needs Kubernetes 1.28 or later; older clusters get a clear error.
- No new dependencies.
- Chart values stay backward compatible: an install that sets OIDC values renders the same console as in 0.4.2.

## Interfaces

- `POST /auth/token`: a form-encoded `token`. It returns 303 to `/apps` on success, and 303 to `/login?error=token` on an invalid, anonymous or oversized token. It returns 303 to `/login?error=cluster` when the API server cannot be reached or does not serve SelfSubjectReview.
- `GET /api/me`:
  - Unauthenticated, it gains `tokenSignIn: true`, and `connectors` is empty when OIDC is off.
  - Authenticated, it gains `method: "token" | "oidc"`.
- The login page has a "Sign in with a Kubernetes token" form with a password-type input and a hint showing `kubectl create token <serviceaccount> -n <namespace>`. OIDC buttons are shown only when configured.
- Flags: `--oidc-issuer-url` and `--oidc-client-id` become optional. Setting only one of them is an error at start.
- Chart: `console.oidc.issuerURL` and `console.oidc.clientID` become optional, and must be set together.

## Data

The session cookie (`ksync_session`, sealed as today) gains:

- `m`, the method: `token` or `oidc`;
- `t`, the bearer token, only when the method is `token`.

Nothing is stored server-side. The reader cache keys a token session by the username, the groups and a SHA-256 of the token, so two tokens never share a reader.

## Edge cases

- **Token with surrounding whitespace or a `Bearer ` prefix:** trimmed and accepted.
- **Non-JWT token, for example a static token file:** the session lasts 8 hours.
- **JWT whose `exp` is already past:** refused.
- **SelfSubjectReview succeeds but returns `system:anonymous`:** refused. This happens when anonymous auth is on and the token is garbage.
- **Token of a ServiceAccount with no RBAC:** the sign-in succeeds, and pages show the same "no access" empty states as today.
- **A cookie from a token session presented while OIDC is configured, and the reverse:** both are handled by their method; a cookie without `m` is treated as `oidc`, which keeps 0.4.x sessions working.
- **The same person signs in twice with different tokens:** separate reader cache entries.

## Failure modes

- **API server unreachable or slow at sign-in:** the SelfSubjectReview times out after 10 s and the user is sent to `/login?error=cluster`. No session is created.
- **A token expires or is revoked mid-session:** reads return 401. The API client maps that to the existing behaviour of sending the browser to `/login`, and the session cookie is cleared.
- **SelfSubjectReview not served (a cluster older than 1.28):** `/login?error=cluster`, and the console logs a message naming the requirement.
- **Misconfigured OIDC, with only an issuer or only a client ID:** the console refuses to start, with a message.

## Acceptance criteria

- [ ] [S-1] [S-4] `TestTokenSignIn` (envtest) fails if a valid ServiceAccount token does not produce a session that reports that ServiceAccount's username via `/api/me`. It also fails if `system:anonymous`, an expired JWT, a malformed token or a token over 16 KiB gets a session.
- [ ] [S-2] `TestTokenSessionReadsAsTheToken` fails if any request in a token session carries the console's own credentials or an `Impersonate-*` header. It also fails if a write, a Secret read or a subresource request leaves the process.
- [ ] [S-2] `TestTokenSessionFollowsRBAC` (envtest) fails if a token with no access to a namespace sees its Applications, or if a token with access does not.
- [ ] [S-3] `TestConsoleStartsWithoutOIDC` fails if the console refuses to start without OIDC flags, if `/auth/start` does not answer 404 without OIDC, or if the console accepts only one of the two OIDC flags.
- [ ] [S-5] `TestTokenNeverLeaves` fails if the token appears in a log line, a response body or a URL during sign-in, reads or sign-out. It also fails if the session outlives `min(exp, 8h)`.
- [ ] [S-6] `TestConsoleChartTokenOnly` fails if the chart without OIDC values renders an impersonate ClusterRole or ClusterRoleBinding, requires a redirect URL, or passes OIDC flags. It also fails if the chart with OIDC values renders differently from 0.4.2.
- [ ] [S-1] [S-3] `TestLoginPage` (Playwright) fails if the token form is missing, if OIDC buttons show without OIDC configured, or if a token sign-in against hack/console-dev does not land on `/apps`.
- [ ] [S-7] `TestConsoleDocsCoverTokenSignIn` fails if `docs/console.md` lacks the token sign-in, viewer-token or raw-manifest sections, and `make docs-check` reports 0 broken links.

## Open questions

<!-- none: all decisions made with the user on 2026-09-25 (token sign-in, OIDC kept optional, kw on token sign-in) -->
