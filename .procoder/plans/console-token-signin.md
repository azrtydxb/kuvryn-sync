# console-token-signin — implementation plan

Status: done
Spec: .procoder/specs/console-token-signin.md

## Goal

Let people sign in to `ksync console` with a Kubernetes bearer token, as the
default that works on a vanilla cluster, and make OIDC an optional extra.

## Architecture

`POST /auth/token` validates a pasted token with an
`authentication.k8s.io/v1` SelfSubjectReview sent with that token through a
dedicated client that may only make that one POST, then seals the token into
the existing AES-256-GCM session cookie next to the identity (`m`, `t`). A
token session's reads go through a new client built from
`rest.AnonymousClientConfig(base)` plus the user's bearer token, wrapped in
`readOnlyTransport` in a token mode that refuses every `Impersonate-*` header
and any `Authorization` other than the session's own token. OIDC routes,
flags, chart RBAC and the impersonation self-check only exist when the OIDC
issuer and client ID are both set.

## Constraints

Copied from the spec:

- The console must never read as its own ServiceAccount in a token session.
- The existing CSP, cookie flags (HttpOnly, Secure unless `--insecure-cookies`, SameSite=Lax) and cross-origin protection apply to `POST /auth/token`.
- Tokens larger than 16 KiB are refused before any request is made.
- `SelfSubjectReview` needs Kubernetes 1.28 or later; older clusters get a clear error.
- No new dependencies.
- Chart values stay backward compatible: an install that sets OIDC values renders the same console as in 0.4.2.

Also, from the maintainer's instructions for this feature:

- Token sign-in is always available; OIDC stays optional; no local users.
- The SelfSubjectReview has a 10 s timeout.
- `system:anonymous` and the `system:unauthenticated` group are refused;
  `system:serviceaccount:*` users are allowed in token mode only. OIDC
  identity rules do not change.
- A cookie without `m` is an OIDC session (0.4.x cookies keep working).
- The session expiry is `min(JWT exp, sign-in + 8h)`; an expired JWT is
  refused; whitespace and a `Bearer ` prefix are trimmed.
- The reader cache key for a token session is username + groups +
  `sha256(token)`.
- The token never appears in a log line, a response body or a URL.
- Every named test fails first, and the failure is recorded in the task's
  todo evidence.
- Commits use plain imperative subjects with a why-body and no attribution.
  Never use local Docker; never touch the kw cluster.

## Task 1: Make OIDC optional in the console configuration and server

Files:

- `internal/console/config.go`: `func (c Config) OIDCEnabled() bool` and
  `func (c Config) Validate() error`.
- `internal/console/auth.go`: `NewAuth` accepts a config without OIDC;
  discovery only runs when OIDC is enabled.
- `internal/console/server.go`: route registration, `/api/me`, `/healthz`.
- `internal/console/selfcheck.go`: skip the impersonation check without OIDC.
- `internal/cli/console.go`: validate flags before starting.
- `internal/console/oidc_optional_test.go`: `TestConsoleStartsWithoutOIDC`.

Interfaces:

- `func (c Config) OIDCEnabled() bool` — true when `IssuerURL != "" || ClientID != ""`.
- `func (c Config) Validate() error` — refuses exactly one of issuer or
  client ID ("--oidc-issuer-url and --oidc-client-id must be set together"),
  OIDC without `--redirect-url`, and `--insecure-cookies` unless
  `--redirect-url` (with OIDC) or `--listen` (without) is on a loopback host.
- `func NewAuth(ctx context.Context, cfg Config, base *rest.Config) (*Auth, error)`.
- `/api/me` gains `tokenSignIn bool`, `oidc bool` and, when authenticated,
  `method string` (`token` or `oidc`).
- `/healthz` reports `"oidc":"disabled"` and `"impersonation":"disabled"`
  without OIDC, and answers 200.

- [x] Write `TestConsoleStartsWithoutOIDC`: `NewAuth` with only a session
      setup and no OIDC flags returns no error; the handler answers 404 to
      `GET /auth/start` and `GET /auth/callback`; `/api/me` returns
      `"connectors":[]`, `"tokenSignIn":true`, `"oidc":false`; `/healthz`
      answers 200; `Config{IssuerURL: "https://x"}.Validate()` and
      `Config{ClientID: "x"}.Validate()` both fail.
- [x] Run `go test ./internal/console -run TestConsoleStartsWithoutOIDC` and
      record the failure (compile errors or a 200 SPA page for /auth/start).
- [x] Implement `OIDCEnabled`, `Validate`, the optional discovery in
      `NewAuth`, the 404 handlers for `/auth/start` and `/auth/callback`
      without OIDC, the `/api/me` fields and the `/healthz` states;
      `SelfCheck` stores `"disabled"` and returns nil without OIDC.
- [x] Update `internal/cli/console.go` to call `cfg.Validate()` and pass the
      rest config to `NewAuth`; update existing tests to the new signature.
- [x] Run `go test ./internal/console ./internal/cli` — expected: PASS.

## Task 2: Read the cluster with the session's own token

Files:

- `internal/console/kube.go`: `TokenClient` and the token mode of
  `readOnlyTransport`.
- `internal/console/token_client_test.go`: `TestTokenSessionReadsAsTheToken`.

Interfaces:

- `func tokenConfig(base *rest.Config, token string) *rest.Config` —
  `rest.AnonymousClientConfig(base)` with `BearerToken = token` and the
  10 s timeout cap.
- `func TokenClient(base *rest.Config, scheme *runtime.Scheme, id Identity) (client.Reader, error)`.
- `readOnlyTransport{next, user, token string}` — with `token` set, the
  request must carry `Authorization: Bearer <token>` exactly and no header
  starting with `Impersonate-`; GET-only, `forbiddenPath`, watch and
  upgrade refusals apply unchanged.
- New error `ErrNotTheSessionToken`.

- [x] Write `TestTokenSessionReadsAsTheToken` against an `httptest` TLS
      server that requests client certificates and records every request.
      The base config carries a bearer token, a bearer token file, a client
      certificate and key, basic auth, `Impersonate`, an `ExecProvider`, an
      `AuthProvider` and a `WrapTransport` that adds an `Impersonate-User`
      header. Assert every recorded request carries only the user's
      bearer token (`user-token`), no `Impersonate-*` header and no peer certificate; assert
      that a Create, a Secret Get and a `pods/log` GET return
      `ErrWriteRefused`/`ErrForbiddenPath` and never reach the server; assert
      the transport alone refuses a request with an `Impersonate-User`
      header or another bearer token.
- [x] Run `go test ./internal/console -run TestTokenSessionReadsAsTheToken`
      and record the failure (undefined `TokenClient`).
- [x] Implement `tokenConfig`, `TokenClient` and the token mode.
- [x] Run the test again — expected: PASS; run
      `go test ./internal/console -run 'TestUserClient|TestForbiddenPaths|TestConsoleClientIsReadOnly'` — expected: PASS.

## Task 3: Sign in with a token and keep it in the session cookie

Files:

- `internal/console/token.go`: the SelfSubjectReview client, token
  normalisation, JWT expiry, identity rules and the `/auth/token` handler.
- `internal/console/auth.go`: `Identity` gains `Method` and `Token`; the
  cookie is sealed through a `sessionData` struct.
- `internal/console/server.go`: route `POST /auth/token`; `withIdentity` and
  `me` check identities by method.
- `internal/console/token_test.go`: `TestTokenSignIn` (envtest),
  `TestTokenNeverLeaves`, `TestTokenSessionCookieFitsInABrowser`,
  `TestTokenSignInRefusesCrossSiteRequests`,
  `TestOIDCCookieWithoutMethodStillWorks`.

Interfaces:

- `type Secret string` with `String()`, `GoString()` and `MarshalJSON()`
  returning `[redacted]`, so an `Identity` printed or encoded never shows
  the token.
- `Identity{Username, Groups, Expiry, Method string, Token Secret}`;
  `MethodToken = "token"`, `MethodOIDC = "oidc"`.
- `func (a *Auth) SignInWithToken(w http.ResponseWriter, r *http.Request)` —
  303 to `/apps`, `/login?error=token` or `/login?error=cluster`.
- `func normalizeToken(raw string) (string, error)` — trims whitespace and a
  case-insensitive `Bearer ` prefix; refuses empty, over 16 KiB, and any
  whitespace or control character inside.
- `func tokenExpiry(token string, now time.Time) (time.Time, error)` —
  `min(exp, now+8h)`; refuses an expired JWT.
- `func checkTokenIdentity(username string, groups []string) error`.
- `func (id Identity) check() error` — token or OIDC rules by `Method`.

- [x] Write `TestTokenSignIn` against envtest: create ServiceAccount
      `a/viewer`, mint a token with TokenRequest, POST it (with surrounding
      whitespace and `Bearer `) to `/auth/token`; expect 303 to `/apps` and a
      session cookie; `/api/me` with the cookie reports
      `system:serviceaccount:a:viewer` and `"method":"token"`. Expect 303 to
      `/login?error=token` and no cookie for: a garbage token, an unsigned
      JWT whose `exp` has passed, a 16 KiB + 1 token, a token with a space
      inside, and a review that answers `system:anonymous` (a fake API
      server). Expect `/login?error=cluster` for an API server that is down
      and one that answers 404.
- [x] Write `TestTokenNeverLeaves`: sign in, read `/api/me`,
      `/api/applications?namespace=a` and sign out with a log sink capturing
      every line; the token must not appear in any log line, response body,
      `Location` header or cookie value in clear. The session expiry equals
      the JWT `exp` when it is under 8 h and sign-in + 8 h otherwise; after
      that instant `Identity` returns `ErrNoSession`.
- [x] Write `TestTokenSessionCookieFitsInABrowser`: a real envtest SA token
      with a 63-character namespace and name gives a sealed cookie under
      4000 bytes, and a token whose cookie would exceed it is refused with
      `error=token`.
- [x] Write `TestTokenSignInRefusesCrossSiteRequests`: a POST with
      `Sec-Fetch-Site: cross-site` gets 403 and no cookie.
- [x] Write `TestOIDCCookieWithoutMethodStillWorks`: a cookie sealed from
      `{u,g,e}` only (the 0.4.x shape) signs in as OIDC; a token cookie with
      an OIDC system identity is refused only by the rules of its method.
- [x] Run `go test ./internal/console -run 'TestTokenSignIn|TestTokenNeverLeaves|TestTokenSessionCookie|TestOIDCCookieWithoutMethod'`
      and record the failures.
- [x] Implement `token.go`, the session changes and the route. The review
      client is `kubernetes.NewForConfig(tokenConfig(base, token))` with a
      `WrapTransport` that allows only
      `POST /apis/authentication.k8s.io/v1/selfsubjectreviews`, with a 10 s
      context timeout. 401/403 map to `token`; 404, timeouts and connection
      errors map to `cluster`; 404 logs "SelfSubjectReview is not served;
      token sign-in needs Kubernetes 1.28 or later". Logs never include the
      token or the raw client error text unredacted.
- [x] Run the tests again — expected: PASS.

## Task 4: Give each token its own reader and follow its RBAC

Files:

- `internal/console/readers.go`: `readerKey` includes the method and, for
  token sessions, `sha256(token)`.
- `internal/console/server.go`: the reader builder dispatches on `Method`;
  an API 401 in a token session clears the session cookie.
- `internal/console/token_rbac_test.go`: `TestTokenSessionFollowsRBAC`,
  `TestReaderCacheSeparatesTokens`.

Interfaces:

- `func readerKey(id Identity) string` — JSON of
  `{"m":…,"u":…,"g":[sorted],"h":hex(sha256(token))}`.
- `type sessionClearer interface{ ClearSession(w http.ResponseWriter) }`
  implemented by `*Auth`.

- [x] Write `TestTokenSessionFollowsRBAC` (envtest): ServiceAccounts
      `a/reader` (RoleBinding to the seeded `kuvryn-sync-viewer` Role in `a`)
      and `a/nobody`; after token sign-in, `reader` sees `web` in `a` and gets
      403 for `b`; `nobody` gets 403 for `a`.
- [x] Write `TestReaderCacheSeparatesTokens`: two identities with the same
      username and groups and different tokens get two builds; the key never
      contains the token.
- [x] Run both and record the failures.
- [x] Implement the key, the dispatch and the 401 cookie clearing.
- [x] Run `go test ./internal/console` — expected: PASS.

## Task 5: Offer token sign-in on the login page and in console-dev

Files:

- `web/src/api/types.ts`: `Me` gains `tokenSignIn`, `oidc`, `method`.
- `web/src/pages/Login.tsx`, `web/src/pages/login.css`: the token form.
- `web/src/api/client.ts`: a 401 posts nothing and sends the browser to
  `/login` (the server already cleared the cookie).
- `hack/console-dev/main.go`, `hack/console-dev/seed.go`: real token sign-in
  next to the stub viewer, a `console-viewer` ServiceAccount, and a `token`
  subcommand.
- `web/e2e/login.spec.ts`: `TestLoginPage` token form, no OIDC buttons, and
  a token sign-in landing on `/apps`.

Interfaces:

- `go run ./hack/console-dev token` prints a fresh TokenRequest token for
  `default/console-viewer` using `hack/console-dev/.kubeconfig`.
- console-dev's authenticator embeds `*console.Auth` (token only) and falls
  back to the stub `viewer` identity when no session cookie is valid.

- [x] Extend `web/e2e/login.spec.ts`: the page shows a "Sign in with a
      Kubernetes token" form with a password input and the hint
      `kubectl create token <serviceaccount> -n <namespace>`; no "Sign in
      with" OIDC button while `/api/me` says `oidc: false`; with `/api/me`
      routed to `oidc: true` and connectors, the OIDC buttons show; a token
      from `go run ./hack/console-dev token` signs in and lands on `/apps`
      showing `system:serviceaccount:default:console-viewer`; a bad token
      lands on `/login?error=token` with its message.
- [x] Run `make test-ui` and record the failure.
- [x] Implement the form with the vendored Azrty `Input` and `Button`
      components, the error texts for `token` and `cluster`, and the
      console-dev changes.
- [x] Run `make test-ui` — expected: all Playwright tests pass; run
      `npm --prefix web run typecheck` and `npm --prefix web test` — PASS.

## Task 6: Render a token-only console from the chart

Files:

- `charts/kuvryn-sync/templates/console.yaml`, `charts/kuvryn-sync/values.yaml`.
- `internal/controller/rbac_manifest_test.go`: `TestConsoleChartTokenOnly`.
- `internal/controller/testdata/console-0.4.2/*.yaml`: golden renders of
  v0.4.2's chart.

Interfaces:

- Values unchanged in shape; `console.oidc.issuerURL` and
  `console.oidc.clientID` become optional and must be set together.

- [x] Produce the goldens by rendering `git show v0.4.2:charts/kuvryn-sync`
      (extracted to a scratch directory) with the in-process renderer for
      three value sets (minimal OIDC; OIDC with ingress, client secret,
      session key, prefixes, connectors and impersonation lists; OIDC with
      replicas 2), all with `image.tag=v0.4.2`, keeping only the console
      objects.
- [x] Write `TestConsoleChartTokenOnly`: with only `console.enabled=true`
      the render succeeds and has no `ClusterRole`/`ClusterRoleBinding` named
      `…-console`, no `--oidc-`, `--redirect-url` or `--username-claim`
      argument; only an issuer, or only a client ID, fails with
      "must be set together"; each OIDC value set renders byte-identical
      console objects to its golden.
- [x] Run `go test ./internal/controller -run TestConsoleChart` and record
      the failure.
- [x] Change the template: `$oidc := or issuerURL clientID`; fail unless
      both are set; wrap the ClusterRole, ClusterRoleBinding, OIDC args,
      claim args and redirect requirement in `if $oidc`.
- [x] Run `go test ./internal/controller` and `helm lint charts/kuvryn-sync` — PASS.

## Task 7: Document token sign-in

Files:

- `docs/console.md`, `docs/security.md`, `CHANGELOG.md`.
- `internal/console/docs_test.go`: `TestConsoleDocsCoverTokenSignIn`.

Interfaces: none.

- [x] Write `TestConsoleDocsCoverTokenSignIn`: `docs/console.md` has the
      headings `## Sign in with a Kubernetes token`,
      `## Create a viewer token` and `## Add the console to a raw-manifest install`; the viewer
      section contains `kind: ServiceAccount`, `kind: RoleBinding` and
      `kubectl create token`; the raw-manifest section contains
      `--show-only templates/console.yaml`; the token section comes before
      `## Set up Dex`; `docs/security.md` mentions token sessions.
- [x] Run it and record the failure.
- [x] Rewrite `docs/console.md`: token sign-in first, OIDC optional, the
      viewer-token recipe with a dedicated read-only role (the generated
      `kuvryn-sync-*-viewer-role` ClusterRoles exist only in
      `dist/install.yaml`, one per kind, and do not aggregate to `view`), the
      raw-manifest recipe, updated values table, flags and troubleshooting.
      Update `docs/security.md`; add `## Unreleased` to `CHANGELOG.md`.
- [x] Run the test and `make docs-check` — PASS, 0 broken links.

## Task 8: Prepare release 0.5.0 and verify

Files:

- `CHANGELOG.md`, `charts/kuvryn-sync/Chart.yaml`, `dist/install.yaml`.

Interfaces: none.

- [x] Rename `## Unreleased` to `## 0.5.0` with an italic one-line summary.
- [x] Set `version` and `appVersion` to `0.5.0` in `Chart.yaml`.
- [x] `make build-installer IMG=ghcr.io/azrtydxb/kuvryn-sync:v0.5.0`, then
      `launcher.sh format dist/install.yaml > dist/install.yaml.formatted && mv dist/install.yaml.formatted dist/install.yaml`.
- [x] Run `make test`, `make lint`, `make test-ui`, `make docs-check`,
      `helm lint charts/kuvryn-sync`, `launcher.sh check --paths-from <diff list>`
      and `launcher.sh release 0.5.0` — all pass or report ready.
- [x] Security self-review of the diff (token leaks, credential
      bleed-through, cookie size, CSRF on `/auth/token`, log redaction); fix
      findings; commit; `git push -u origin feat/console-token-signin`; no PR.

## Deviations from the plan

- Task 3 also routed token sessions to `TokenClient` in the reader builder,
  since sign-in could not be exercised end to end without it. Task 4 kept the
  cache key and the 401 handling, and its RBAC test proved per-token
  isolation by revoking one bound token.
- `/api/me` also reports `oidc`, because an empty connector list cannot tell
  "OIDC off" from "OIDC with no Dex connectors".
- Token identities also refuse `system:` users other than ServiceAccounts,
  such as node and bootstrap tokens.
- `--insecure-cookies` without OIDC requires a loopback `--listen`, which
  mirrors the redirect-URL rule it has with OIDC.
- The chart goldens are compared by content, so they can be formatted. The
  helm CLI output for OIDC values is still byte-identical to v0.4.2's.
- The security review added origin pinning for every console request and
  the sign-in review, so a redirect cannot carry the token to another host.
