# kuvryn-sync-console — implementation plan

Status: draft
Spec: .procoder/specs/kuvryn-sync-console.md

## Goal

Ship `ksync console`, a read-only web console that signs users in with OIDC
and shows Kuvryn Sync state through their own Kubernetes RBAC, implementing
the claude.ai/design Login and Console designs.

## Architecture

A new Go package `internal/console` holds four pieces:

- the HTTP server;
- the OIDC login with an encrypted session cookie;
- an impersonating, read-only Kubernetes client built per request from the
  session identity;
- a JSON API that maps CRDs to small view models.

A React + Vite + TypeScript SPA under `web/` uses a vendored copy of the Azrty
design system, polls the JSON API every 10 seconds, and is built into
`internal/console/ui/dist`, which Go embeds; a committed placeholder page
stands in when no build exists. The chart adds an optional console Deployment
whose ServiceAccount may only impersonate users and groups.

## Constraints

- This plan runs after the kuvryn-sync-rebrand plan. It uses:
  - the CLI `ksync` and the API group `sync.kuvryn.io`;
  - the chart `charts/kuvryn-sync` and the namespace `kuvryn-sync-system`;
  - the module `github.com/azrtydxb/kuvryn-sync`.
- It is read-only. The console client only ever sends get, list and watch,
  and never reads Secrets.
- Every cluster read impersonates the session user and groups. Usernames or
  groups starting with `system:` are refused.
- OIDC uses the authorization code flow with PKCE (S256), plus state and
  nonce, and the ID token is verified against the issuer's JWKS.
- The session cookie is named `ksync_session`. It is AES-256-GCM encrypted
  with the key from `--session-key-file` (32 bytes), is HttpOnly, Secure
  (unless `--insecure-cookies`) and SameSite=Lax, and expires at the ID
  token's `exp`. No refresh tokens are stored.
- The username claim defaults to `email` and the groups claim to `groups`.
  Both prefixes are empty by default.
- The UI refreshes every 10 seconds by polling. There are no server-side
  watches.
- Namespaces: try a cluster-wide list first. On 403, the UI shows a picker
  of the namespaces the user may list, with typed entry as the fallback. The
  choice is kept in localStorage under `ksync.namespace`.
- `web/dist` output is never committed. The Dockerfile and CI build it, and
  the committed `internal/console/ui/placeholder/index.html` is embedded when
  there is no build.
- The design follows the claude.ai/design project
  `c45e001b-cc5a-4b61-8f11-122044379a06`:
  - pages: "Kuvryn Sync Login.dc.html" and "Kuvryn Sync Console.dc.html";
  - Azrty design system `c1ca7a31-5e20-46fb-8f31-9f912b546f68`;
  - pillar `build`, dark default, light supported;
  - copy taken verbatim from the design, with "solder" renamed to "ksync"
    and `solder.io/v1alpha1` to `sync.kuvryn.io/v1alpha1`.
- There is a strict CSP: `default-src 'self'; img-src 'self' data:;
style-src 'self'; script-src 'self'; frame-ancestors 'none'`.
- Commits use imperative subjects of 72 characters or fewer, with a
  why-body and no attribution.

## Task 1: Serve ksync console with health and an embedded placeholder

Files:

- `internal/console/config.go`: the `Config` struct and flag binding.
- `internal/console/server.go`: `NewServer(cfg Config, base *rest.Config) (*Server, error)`
  and `(*Server).Handler() http.Handler`, which serves `/healthz`, the
  security headers and the SPA fallback.
- `internal/console/ui/embed.go`: `//go:embed all:dist all:placeholder` and
  `func Files() fs.FS`, returning `dist` when `dist/index.html` exists and
  `placeholder` otherwise.
- `internal/console/ui/placeholder/index.html`: a page explaining
  `make web-build`.
- `internal/console/ui/dist/.gitkeep`, with `.gitignore` covering
  `internal/console/ui/dist/*` except `.gitkeep`.
- `internal/cli/plan.go`: `case "console":` calls `runConsole`.
- `internal/cli/console.go`: `runConsole(ctx, args, stdout, stderr) error`
  parses flags into `console.Config` and runs the server.
- `internal/console/server_test.go`.

Interfaces: produces

- `console.Config{Listen, IssuerURL, ClientID, ClientSecretFile, RedirectURL, UsernameClaim, GroupsClaim, UsernamePrefix, GroupsPrefix, SessionKeyFile, ClusterName, Connectors []string, DocsURL, StatusURL string; InsecureCookies bool}`;
- the flags `--listen` (default `:8080`), `--oidc-issuer-url`,
  `--oidc-client-id`, `--oidc-client-secret-file`, `--redirect-url`,
  `--username-claim` (default `email`), `--groups-claim` (default `groups`),
  `--username-prefix`, `--groups-prefix`, `--session-key-file`,
  `--cluster-name` (default `cluster`), `--connectors`, `--docs-url`,
  `--status-url` and `--insecure-cookies`.

- [ ] Write `internal/console/server_test.go`:
  ```go
  func TestServerServesHealthAndSecurityHeaders(t *testing.T) {
  	s, err := NewServer(Config{ClusterName: "test"}, &rest.Config{Host: "https://127.0.0.1:1"})
  	if err != nil {
  		t.Fatal(err)
  	}
  	rec := httptest.NewRecorder()
  	s.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/apps", nil))
  	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "<html") {
  		t.Fatalf("SPA fallback: %d %s", rec.Code, rec.Body.String())
  	}
  	if got := rec.Header().Get("Content-Security-Policy"); !strings.Contains(got, "frame-ancestors 'none'") {
  		t.Fatalf("CSP = %q", got)
  	}
  	rec = httptest.NewRecorder()
  	s.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/healthz", nil))
  	if rec.Code != 503 {
  		t.Fatalf("healthz before OIDC discovery = %d, want 503", rec.Code)
  	}
  }
  ```
  Run `go test ./internal/console/`, and expect it to FAIL to compile with
  "undefined: NewServer".
- [ ] Implement the files listed under Files. `/healthz` returns 503 with a
      JSON body `{"oidc":"pending","impersonation":"unchecked"}` until Task 2
      and Task 8 set them. Unknown paths serve `index.html` from `ui.Files()`
      with `Content-Type: text/html`, and `/assets/*` serves files with long
      cache headers.
- [ ] Run `go test ./internal/console/ ./internal/cli/`, and expect PASS.
      `ksync console --help` prints every flag.
- [ ] Commit "Serve ksync console with health and an embedded placeholder".

## Task 2: Sign users in with OIDC and an encrypted session

Files:

- `internal/console/auth.go`:
  - `type Identity struct{ Username string; Groups []string; Expiry time.Time }`;
  - `type Auth struct` with `NewAuth(ctx, cfg Config) (*Auth, error)`;
  - `(*Auth).Start(w, r)` for `GET /auth/start?connector=`;
  - `(*Auth).Callback(w, r)` for `GET /auth/callback`;
  - `(*Auth).Logout(w, r)` for `POST /logout`;
  - `(*Auth).Identity(r) (Identity, error)`.
- `internal/console/session.go`: `seal(key []byte, v any) (string, error)` and
  `open(key []byte, s string, v any) error`, using AES-256-GCM with a random
  nonce and base64url encoding.
- `internal/console/server.go`: wires the routes and `/api/me`.
- `internal/console/auth_test.go`: the test issuer and the tests.
- `go.mod`: `github.com/coreos/go-oidc/v3` and `golang.org/x/oauth2`.

Interfaces: consumes `Config` from Task 1. Produces `Identity`, the cookies
`ksync_session`, `ksync_state` and `ksync_nonce` (the verifier and state
live in `ksync_state`, which expires after 10 minutes), and `ErrNoSession`.
It also produces `type Authenticator interface{ Identity(*http.Request) (Identity, error) }`
and `(*Server).UseAuthenticator(Authenticator)`: `*Auth` implements it and
adds the sign-in routes, while Task 4's fake and Task 7's stub `Auth` are
plain `Authenticator`s. A failed sign-in step answers with its status code
(400, 403 or 503) and a small page that returns the browser to
`/login?error=<reason>` for the "Sign-in failed" alert. When the username
claim is `email`, a token whose `email_verified` is false is refused with 403.

- [ ] Write `internal/console/auth_test.go`. It holds an in-process issuer
      (`newTestIssuer(t)`) that serves discovery and JWKS, and signs RS256 ID
      tokens with `github.com/go-jose/go-jose/v4`, which is already an indirect
      dependency:
  ```go
  func TestOIDCLoginFlow(t *testing.T) {
  	iss := newTestIssuer(t) // serves /.well-known/openid-configuration, /keys, /token
  	a := mustAuth(t, iss.URL)
  	start := httptest.NewRecorder()
  	a.Start(start, httptest.NewRequest("GET", "/auth/start?connector=github", nil))
  	loc, _ := url.Parse(start.Header().Get("Location"))
  	q := loc.Query()
  	if q.Get("code_challenge_method") != "S256" || q.Get("connector_id") != "github" || q.Get("state") == "" || q.Get("nonce") == "" {
  		t.Fatalf("authorize URL lacks PKCE/state/nonce/connector: %s", loc)
  	}
  	iss.issueFor(q.Get("nonce"), "alice@acme.io", []string{"team-a"})
  	cb := httptest.NewRequest("GET", "/auth/callback?code=c&state="+q.Get("state"), nil)
  	for _, c := range start.Result().Cookies() {
  		cb.AddCookie(c)
  	}
  	rec := httptest.NewRecorder()
  	a.Callback(rec, cb)
  	var session *http.Cookie
  	for _, c := range rec.Result().Cookies() {
  		if c.Name == "ksync_session" {
  			session = c
  		}
  	}
  	if session == nil || !session.HttpOnly || !session.Secure || session.SameSite != http.SameSiteLaxMode {
  		t.Fatalf("session cookie = %+v", session)
  	}
  	bad := httptest.NewRequest("GET", "/auth/callback?code=c&state=wrong", nil)
  	for _, c := range start.Result().Cookies() {
  		bad.AddCookie(c)
  	}
  	rec = httptest.NewRecorder()
  	a.Callback(rec, bad)
  	if rec.Code != http.StatusBadRequest {
  		t.Fatalf("wrong state accepted: %d", rec.Code)
  	}
  	iss.issueFor("other-nonce", "alice@acme.io", nil)
  	rec = httptest.NewRecorder()
  	a.Callback(rec, cb)
  	if rec.Code != http.StatusBadRequest {
  		t.Fatalf("wrong nonce accepted: %d", rec.Code)
  	}
  	iss.issueUnsigned("alice@acme.io")
  	rec = httptest.NewRecorder()
  	a.Callback(rec, cb)
  	if rec.Code != http.StatusBadRequest {
  		t.Fatalf("unverifiable ID token accepted: %d", rec.Code)
  	}
  }
  ```
  Run `go test ./internal/console/ -run TestOIDCLoginFlow`, and expect it to
  FAIL to compile with "undefined: newTestIssuer".
- [ ] Implement the test issuer, then `auth.go` and `session.go`.
  - **Callback:** it verifies the state against the cookie, exchanges the
    code with the PKCE verifier, and verifies the ID token with
    `oidc.Provider.Verifier(&oidc.Config{ClientID})`. It checks that the
    nonce matches the cookie, reads the claims named by `UsernameClaim` and
    `GroupsClaim`, and applies the prefixes.
  - **Refusals:** an empty username returns a 400 naming the claim. A
    `system:` username or group returns 403.
  - **Session:** more than 4000 bytes returns 400 "too many groups for a
    session". Otherwise it sets `ksync_session` with an expiry equal to the
    token's `exp`.
  - **Discovery:** it retries every 10 seconds in the background until it
    succeeds, and `/healthz` reports `"oidc":"ready"` once it has.
- [ ] Run `go test ./internal/console/`, and expect PASS.
- [ ] Commit "Sign console users in with OIDC and an encrypted session".

## Task 3: Read the cluster as the user, read-only

Files:

- `internal/console/kube.go`:
  - `func UserClient(base *rest.Config, scheme *runtime.Scheme, id Identity) (client.Reader, error)`
    copies `base`, sets `Impersonate{UserName, Groups}`, and wraps the
    transport in `readOnlyTransport`;
  - `readOnlyTransport` returns an error for any method other than GET
    before sending, and refuses any path under `/api/v1/namespaces/*/secrets`
    or `/api/v1/secrets`.
- `internal/console/kube_test.go`.

Interfaces: consumes `Identity`. Produces `UserClient` and
`ErrWriteRefused`, plus `ErrForbiddenPath` and `ErrNotImpersonated`. client-go
applies `WrapTransport` inside its impersonation wrapper, so
`readOnlyTransport` sees the final request: besides refusing non-GETs and
Secret paths, it refuses a request whose `Impersonate-User` is missing, a
`system:` identity or not the session user, a `system:` `Impersonate-Group`,
any `Upgrade` header, and the `proxy`, `exec`, `attach`, `portforward` and
`log` subresources.

- [ ] Write `internal/console/kube_test.go`:
  ```go
  func TestConsoleClientIsReadOnly(t *testing.T) {
  	var sent []string
  	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
  		sent = append(sent, r.Method+" "+r.URL.Path)
  		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{}`)), Header: http.Header{"Content-Type": {"application/json"}}}, nil
  	})
  	for _, method := range []string{"POST", "PUT", "PATCH", "DELETE"} {
  		req, _ := http.NewRequest(method, "https://k8s/apis/sync.kuvryn.io/v1alpha1/namespaces/a/applications/x", nil)
  		if _, err := (readOnlyTransport{next: rt}).RoundTrip(req); !errors.Is(err, ErrWriteRefused) {
  			t.Fatalf("%s allowed: %v", method, err)
  		}
  	}
  	req, _ := http.NewRequest("GET", "https://k8s/api/v1/namespaces/a/secrets/db", nil)
  	if _, err := (readOnlyTransport{next: rt}).RoundTrip(req); err == nil {
  		t.Fatal("Secret read allowed")
  	}
  	if len(sent) != 0 {
  		t.Fatalf("requests reached the API server: %v", sent)
  	}
  	if _, err := UserClient(&rest.Config{Host: "https://k8s"}, scheme.Scheme, Identity{Username: "system:admin"}); err == nil {
  		t.Fatal("system: user admitted")
  	}
  	if _, err := UserClient(&rest.Config{Host: "https://k8s"}, scheme.Scheme, Identity{Username: "a", Groups: []string{"system:masters"}}); err == nil {
  		t.Fatal("system: group admitted")
  	}
  }
  ```
  Run `go test ./internal/console/ -run TestConsoleClientIsReadOnly`, and
  expect it to FAIL to compile with "undefined: readOnlyTransport".
- [ ] Implement `kube.go`. The client comes from `client.New` with
      `rest.CopyConfig` and `WrapTransport` set to wrap in `readOnlyTransport`.
      Watch requests are GETs with `?watch=true` and stay allowed, although the
      UI never issues them.
- [ ] Run `go test ./internal/console/`, and expect PASS.
- [ ] Commit "Read the cluster as the console user, read-only".

## Task 4: Serve the read-only JSON API with RBAC-following namespaces

Files:

- `internal/console/api.go`: handlers for
  - `GET /api/me`, returning `{username, groups, cluster, connectors, docsURL, statusURL}`;
  - `GET /api/namespaces`;
  - `GET /api/applications[?namespace=]` and
    `GET /api/applications/{ns}/{name}`;
  - `GET /api/applications/{ns}/{name}/revisions`;
  - `GET /api/applications/{ns}/{name}/resources`;
  - `GET /api/repositories[?namespace=]`, `GET /api/revisions[?namespace=]`
    and `GET /api/imagepolicies[?namespace=]`.
- `internal/console/views.go`: view models `AppRow`, `AppDetail`, `Cause`,
  `PlanView`, `RevisionRow`, `ResourceRow`, `RepoRow` and `ImagePolicyRow`,
  with converter functions from the `v1alpha1` types. Unknown values are
  shown as `"—"`, and every message goes through `redact.String`.
- `internal/console/api_test.go`: envtest with RBAC.

Interfaces: consumes `UserClient` and `Identity`, plus
`applier.ListManaged(ctx, reader, app, applier.ListOptions{MetadataOnly, Exclude})`
from the rebrand code; it returns `(objects, skippedKinds, error)`, and this
task adds the `Exclude` option so the console never lists Secrets at all.
Produces JSON shapes the SPA types mirror in `web/src/api/types.ts`:

- `AppRow{name, namespace, destination, repository, path, render, commit, sync, health, lastReconcile}`;
- `AppDetail{...AppRow, state, desiredRevision, deployedRevision, source{repository, revision, path, render}, policy{automatic, prune, selfHeal, suspend, conflictPolicy, failureAction, deletionPolicy, serviceAccountName}, conditions[{type, status, reason, message, lastTransitionTime}], diagnosis[]Cause, plan PlanView|null, planVisible}`;
- `Cause{resource, reason, message, chain[{kind, name, state}]}`. Status
  records only the chain's references, so only the root link's `state`
  is known: the cause's reason. The other links show `"—"`;
- `PlanView{revision, commit, digest, phase, summary{create, update, delete, unchanged}, truncated, resources[{action, ref{apiVersion, kind, namespace, name}, changes[{path, before, after, redacted}], warnings}]}`.
  Changes are redacted by the same rule as `ksync plan`;
- `RevisionRow{name, namespace, application, commit, phase, plan{create, update, delete, unchanged}, digest, approvedBy, attempts, started, failure}`;
- `ResourceRow{kind, name, apiVersion, sync, health, visible}`;
- `RepoRow{name, namespace, url, ref, observed, state, message, apps, poll, webhook, lastFetch}`,
  where `apps` is `null` when the user may not list Applications;
- `ImagePolicyRow{name, namespace, image, rule, latest, digest, lastScan}`.

A cluster-wide 403 returns `{"error":"forbidden","needNamespace":true}` with
status 403.

- [ ] Write `internal/console/api_test.go`. It starts envtest with
      `APIServer` args `--authorization-mode=RBAC`, installs the CRDs, creates
      Applications `web` in namespace `a` and `db` in namespace `b` and a Secret
      `a/creds`, and binds user `alice` to a Role that allows get and list on
      `applications`, `revisions` and `repositories` in `a` only:
  ```go
  func TestConsoleFollowsUserRBAC(t *testing.T) {
  	srv := newAPIServer(t, env) // Server with a fake Auth returning Identity{Username: "alice"}
  	all := get(t, srv, "/api/applications")
  	if all.Code != 403 || !strings.Contains(all.Body.String(), `"needNamespace":true`) {
  		t.Fatalf("cluster-wide list for a namespaced user = %d %s", all.Code, all.Body)
  	}
  	a := get(t, srv, "/api/applications?namespace=a")
  	if a.Code != 200 || !strings.Contains(a.Body.String(), `"name":"web"`) {
  		t.Fatalf("namespace a = %d %s", a.Code, a.Body)
  	}
  	b := get(t, srv, "/api/applications?namespace=b")
  	if b.Code != 403 || strings.Contains(b.Body.String(), "db") {
  		t.Fatalf("namespace b leaked: %d %s", b.Code, b.Body)
  	}
  }

  func TestConsoleNeverReturnsSecrets(t *testing.T) {
  	srv := newAPIServer(t, env)
  	for _, path := range []string{"/api/me", "/api/namespaces", "/api/applications?namespace=a", "/api/applications/a/web", "/api/applications/a/web/revisions", "/api/applications/a/web/resources", "/api/repositories?namespace=a", "/api/revisions?namespace=a", "/api/imagepolicies?namespace=a"} {
  		body := get(t, srv, path).Body.String()
  		if strings.Contains(body, "creds") || strings.Contains(body, "c2VjcmV0") {
  			t.Fatalf("%s returned Secret data: %s", path, body)
  		}
  	}
  }
  ```
  Run `go test ./internal/console/ -run 'TestConsoleFollowsUserRBAC|TestConsoleNeverReturnsSecrets'`.
  `newAPIServer`, `get` and the fake Auth are test helpers in the same file,
  so it compiles, and expect it to FAIL with "cluster-wide list for a
  namespaced user = 404". `TestConsoleNeverReturnsSecrets` passes trivially
  against 404s, so `TestConsoleAPIShowsWhatTheUserMayRead` pins the content
  each endpoint returns, and a Secret `a/creds` labelled as managed by `web`
  makes any Secret listing leak into the Resources response.
- [ ] Implement `api.go` and `views.go`.
  - **Resources:** the endpoint lists managed objects with
    `MetadataOnly: graph.IdentityOnly`, so ConfigMaps and ServiceAccounts are
    read as metadata only, while workloads are read in full to evaluate their
    health. It never lists Secrets (`Exclude`). `status.managedKinds` records
    kinds, not names, so when the inventory includes Secrets, each Secret the
    newest Revision's plan names becomes a row `{kind:"Secret", name,
visible:false}`, or a single row named `"—"` when no plan names one.
    Kinds the user may not list become `{kind, name:"—", visible:false}`
    rows.
  - **Namespaces:** `/api/namespaces` lists namespaces as the user and
    returns 403 with `{"error":"forbidden"}` when that is not allowed.
  - **Timeouts:** every handler uses a 10-second context timeout, and on
    timeout returns 504 `{"error":"timeout"}`.
- [ ] Run `go test ./internal/console/`, and expect PASS.
- [ ] Commit "Serve the console's read-only API as the signed-in user".

## Task 5: Scaffold the SPA with the vendored Azrty design system

Files:

- `web/package.json`: `react@19`, `react-dom@19`, `react-router-dom@7`,
  `vite@7`, `typescript@5`, `@vitejs/plugin-react@5` (6 needs Vite 8),
  `vitest@5`, `@playwright/test` and `@axe-core/playwright`, with the scripts
  `dev`, `build`, `test`, `test:e2e` and `typecheck`. `web/package-lock.json`
  is committed.
- `web/vite.config.ts`: `build.outDir` is `../internal/console/ui/dist`, and
  `emptyOutDir` is true. A small plugin rewrites the tracked `.gitkeep` that
  `emptyOutDir` deletes, and `assetsInlineLimit: 0` keeps fonts and images
  out of `data:` URIs, which the CSP's `default-src 'self'` would block for
  fonts.
- `web/tsconfig.json` and `web/index.html`.
- `web/src/azrty/`: vendored from design-system project
  `c1ca7a31-5e20-46fb-8f31-9f912b546f68`:
  - `tokens/*.css` and `components/components.css`;
  - `assets/fonts/*.woff2` and `assets/icons/lucide.woff2`;
  - `components/{actions/Button, brand/Icon, brand/Logo, brand/ProductLogo, forms/Input, feedback/Alert, feedback/Badge, feedback/EmptyState, data/StatCard, data/Table, data/CodeBlock, navigation/Tabs, navigation/Topbar, navigation/Sidebar, panels/Drawer}.jsx`
    and their dependencies (IconButton, Select, Avatar, Sparkline). The
    `.d.ts` files were not vendored: `tsconfig.json` sets `allowJs` and
    imports the `.jsx` directly, and the directory is read-only (wrappers go
    in `web/src/ui/`);
  - `README.md` recording the source project ID, file list and sync date.
- `web/src/assets/kuvryn-sync-emblem-dark.png` and
  `web/src/assets/kuvryn-sync-emblem-light.png`: 640x640 PNGs from the
  maintainer; the design tool truncates them. Until they are committed, local
  builds use untracked placeholders listed in `.git/info/exclude`, and the
  image and CI UI builds fail on the missing import.
- `web/src/brand.ts`: the emblem imports and the shared ProductLogo props
  (`name="Kuvryn" sub="Sync" tagline="GitOps that sticks" pillar="build"`).
- `web/src/main.tsx` and `web/src/App.tsx`: the routes `/login`, `/apps`,
  `/apps/:ns/:name/:tab?`, `/repositories`, `/revisions` and
  `/imagepolicies`.
- `web/src/api/client.ts` exports:
  - `getJSON<T>(path)`, which on 401 sets `location.href = "/login"`;
  - `usePoll<T>(path, 10000)`, returning `{data, error, refreshedAt}`.
- `web/src/api/types.ts`: the Task 4 view-model types.
- `web/src/theme.ts`: `useTheme()`, stored in localStorage `ksync.theme`,
  with dark as the default.
- `Makefile`: the targets `web-build` (`npm --prefix web ci && npm --prefix web run build`)
  and `test-ui` (`npm --prefix web run build && npm --prefix web run test:e2e`:
  `go run ./hack/console-dev` embeds `internal/console/ui/dist` at compile
  time, so the UI must be built first).
- `Dockerfile`: a `node:22-alpine` stage that builds `web`, whose output the
  Go stage copies to `internal/console/ui/dist` before `go build`.
  `.dockerignore` re-includes `internal/console/ui/placeholder/**`, the
  dist `.gitkeep` and `web/**` (without `node_modules`), since it otherwise
  admits only `.go` files and `go:embed` would find no placeholder.
- `.github/workflows/test.yml`: `actions/setup-node` (pinned) and a step
  `npm --prefix web ci && npm --prefix web run typecheck && npm --prefix web test && npm --prefix web run build`.

Interfaces: consumes the Task 4 JSON shapes. Produces `usePoll`, `getJSON`,
`useTheme` and the vendored components imported from `web/src/azrty/...`.

- [ ] Write `web/src/api/client.test.ts` (Vitest, added to devDependencies,
      with a `test` script):
  ```ts
  import { describe, expect, it, vi } from "vitest";
  import { getJSON } from "./client";
  describe("getJSON", () => {
    it("sends the browser to /login on 401", async () => {
      vi.stubGlobal(
        "fetch",
        vi.fn().mockResolvedValue(new Response("", { status: 401 })),
      );
      const loc = { href: "/apps" };
      vi.stubGlobal("location", loc);
      await expect(getJSON("/api/applications")).rejects.toThrow(
        "unauthorized",
      );
      expect(loc.href).toBe("/login");
    });
  });
  ```
  Run `npm --prefix web test`, and expect it to FAIL with "Cannot find module
  './client'" (Vitest 5's wording).
- [ ] Vendor the design-system files. Read each file listed under Files with
      DesignSync `get_file` and write it verbatim under `web/src/azrty/`; binary
      files come back as base64. Record the file list in
      `web/src/azrty/README.md`.
- [ ] Implement `client.ts`, `types.ts`, `theme.ts`, `App.tsx` and the Vite
      config. Then run `npm --prefix web test && npm --prefix web run build`,
      and expect PASS, with `internal/console/ui/dist/index.html` created. Run
      `go test ./internal/console/`, and expect PASS: the dist is served when it
      is present.
- [ ] Commit "Scaffold the console SPA on the Azrty design system".

## Task 6: Implement the login page from the design

Files:

- `web/src/pages/Login.tsx`: layout split (≥ 900px wide, with the brand
  panel) or centred.
  - **Header:** the ProductLogo icon (44) and the wordmark "Kuvryn" and
    "Sync".
  - **Intro:** the eyebrow "GitOps for Kubernetes", the heading "Welcome
    back", and the copy "Sign in to view applications, plans and revisions.
    What you can see follows your Kubernetes RBAC."
  - **Sign-in controls:**
    - the SSO button "Sign in with {ssoName}" (icon `key-round`), which
      calls `/auth/start`;
    - secondary buttons for the `github` and `gitlab` connectors, calling
      `/auth/start?connector=github|gitlab`;
    - for the `local` connector, a divider "or with email" and a button
      "Sign in with email", calling `/auth/start?connector=local`. Dex
      hosts the password form, so the console never sees passwords.
  - **Footer copy:** "Read-only access. Changes go through Git or the CLI.",
    then "An Azrty product" with the Logo mark, the Docs and Status links,
    and "{cluster} · v1alpha1".
  - **Error:** the Alert "Sign-in failed", shown when the URL has `?error=`.
  - **Brand panel:** the ProductLogo (210) on the rotated rings, the heading
    "GitOps that sticks." and the copy "Every commit planned, applied and
    explained."
- `web/src/pages/login.css`: layout only; colours come from tokens.
- `internal/console/server.go`: `/login` serves the SPA. Before the session
  exists, `/api/me` returns `{authenticated:false, connectors, cluster, ssoName, docsURL, statusURL}`.
- `web/e2e/login.spec.ts` and `web/playwright.config.ts`. The latter's
  `webServer` is `go run ./hack/console-dev` (Task 7).

Interfaces: consumes `/api/me` (unauthenticated shape), `/auth/start`,
`ProductLogo`, `Button`, `Alert` and `Logo`. Produces the route `/login` and
the test `TestLoginPage` (the `login.spec.ts` suite).

- [ ] Write `web/e2e/login.spec.ts`:
  ```ts
  import { test, expect } from "@playwright/test";
  import AxeBuilder from "@axe-core/playwright";
  for (const theme of ["dark", "light"]) {
    for (const width of [1400, 700]) {
      test(`TestLoginPage ${theme} ${width}px`, async ({ page }) => {
        await page.setViewportSize({ width, height: 900 });
        await page.addInitScript(
          (t) => localStorage.setItem("ksync.theme", t),
          theme,
        );
        await page.goto("/login");
        await expect(
          page.getByRole("heading", { name: "Welcome back" }),
        ).toBeVisible();
        await expect(
          page.getByRole("button", { name: /Sign in with/ }),
        ).toBeVisible();
        await expect(page.getByText("GitOps that sticks.")).toBeVisible({
          visible: width >= 900,
        });
        const results = await new AxeBuilder({ page }).analyze();
        expect(
          results.violations.filter(
            (v) => v.impact === "serious" || v.impact === "critical",
          ),
        ).toEqual([]);
      });
    }
  }
  ```
  Run `make test-ui`, and expect it to FAIL with a timeout finding the
  "Welcome back" heading.
- [ ] Implement `Login.tsx` and `login.css` from "Kuvryn Sync Login.dc.html",
      using the component props that file uses.
- [ ] Run `make test-ui`, and expect all four TestLoginPage cases to pass.
- [ ] Commit "Implement the console login page from the design".

## Task 7: Implement the console pages from the design

Files:

- `hack/console-dev/main.go`: starts envtest with RBAC and the CRDs, and
  seeds:
  - the design's sample Applications: payments (Degraded, with a diagnosis),
    checkout (AwaitingApproval), catalog, search (Drifted), notifications,
    ingress-nginx, cert-manager and ledger;
  - their Revisions, 3 Repositories and 3 ImagePolicies.
    It runs the console with a stub `Auth` that always returns
    `Identity{Username: "viewer", Groups: ["viewers"]}`, bound to a view-all
    ClusterRole, on `:5174`. It prints `ready` when it is serving.
- `web/src/layout/Shell.tsx`: a Sidebar with the brand lockup and cluster,
  and navigation for Applications (with a degraded-count badge),
  Repositories, Revisions and Image policies. The footer shows "Watching
  sync.kuvryn.io/v1alpha1" and "Read-only · viewer RBAC". The Topbar has
  breadcrumbs, "Refreshed hh:mm:ss" and the Read-only badge.
- `web/src/pages/Applications.tsx`: the eyebrow "GitOps", the heading and
  the copy "Sync and health are reported separately. Changes are made in Git
  or with the ksync CLI.", the Degraded alert with "View diagnosis", four
  StatCards (Applications, Healthy, Not synced, Awaiting approval), and the
  table.
- `web/src/pages/ApplicationDetail.tsx` has Tabs:
  - **Overview:** the Source and Sync policy PropertyLists, and Conditions;
  - **Diagnosis:** cause cards with the chain, the EmptyState "No causes
    recorded", and the CodeBlock "Same view from the CLI" containing
    `ksync diagnose {name} -n {ns}` and
    `ksync graph {name} -n {ns} -o dot | dot -Tsvg > {name}.svg`;
  - **Plan:** the summary, the resources table, and for AwaitingApproval
    the CodeBlock with `ksync plan {name} -n {ns}` and
    `ksync sync {name} -n {ns} --revision {revision}`;
  - **History** and **Resources**, where rows the user cannot see show "not
    visible with your permissions".
- `web/src/pages/Repositories.tsx`, `web/src/pages/Revisions.tsx` and
  `web/src/pages/ImagePolicies.tsx`: the design's tables and copy.
- `web/src/components/NamespacePicker.tsx`: shown when an API call returns
  `needNamespace`. It is filled from `/api/namespaces`, falls back to a text
  Input on 403, and stores the choice in localStorage `ksync.namespace`.
- `web/src/components/StatusBadge.tsx`: the sync, health, phase and action
  tone maps from the design's `SYNC_T`, `HEALTH_T`, `PHASE_T` and `ACT_T`.
- `web/e2e/console.spec.ts` and `web/e2e/refresh.spec.ts`.

Interfaces: consumes the Task 4 endpoints, `usePoll`, and the vendored
components. Produces the tests `TestConsolePages` and `TestLiveRefresh`, and
the dev server `go run ./hack/console-dev`, which listens on
`http://localhost:5174`.

- [ ] Write `web/e2e/console.spec.ts`:
  ```ts
  import { test, expect } from "@playwright/test";
  test("TestConsolePages", async ({ page }) => {
    await page.goto("/apps");
    for (const label of [
      "Applications",
      "Healthy",
      "Not synced",
      "Awaiting approval",
    ]) {
      await expect(page.locator(".az-stat", { hasText: label })).toBeVisible();
    }
    await expect(page.getByText("payments is Degraded")).toBeVisible();
    await page.getByRole("button", { name: "View diagnosis" }).click();
    await expect(page.getByText("MissingSecret")).toBeVisible();
    await expect(page.getByText("Chain to root cause")).toBeVisible();
    await expect(page.getByText("ksync diagnose payments")).toBeVisible();
    for (const tab of ["Overview", "Plan", "History", "Resources"]) {
      await page.getByRole("tab", { name: new RegExp(tab) }).click();
      await expect(page.locator("main")).not.toContainText("Error");
    }
    for (const [nav, heading] of [
      ["Repositories", "Repositories"],
      ["Revisions", "Revisions"],
      ["Image policies", "Image policies"],
    ]) {
      await page.getByRole("button", { name: nav }).click();
      await expect(page.getByRole("heading", { name: heading })).toBeVisible();
    }
  });
  ```
  and `web/e2e/refresh.spec.ts`:
  ```ts
  import { test, expect } from "@playwright/test";
  import { execSync } from "node:child_process";
  test("TestLiveRefresh", async ({ page }) => {
    await page.goto("/apps");
    await expect(
      page.getByRole("row", { name: /catalog.*Healthy/ }),
    ).toBeVisible();
    execSync("go run ./hack/console-dev set-health catalog Degraded", {
      cwd: "..",
      stdio: "inherit",
    });
    await expect(
      page.getByRole("row", { name: /catalog.*Degraded/ }),
    ).toBeVisible({ timeout: 11000 });
  });
  ```
  Run `make test-ui`, and expect both to FAIL: the stat cards are not found.
- [ ] Implement `hack/console-dev`, including the subcommand
      `set-health <app> <state>`, which patches the seeded Application's status
      through the envtest kubeconfig the server writes to
      `hack/console-dev/.kubeconfig`. Then implement the pages from "Kuvryn Sync
      Console.dc.html".
- [ ] Run `make test-ui`, and expect TestLoginPage, TestConsolePages and
      TestLiveRefresh to pass.
- [ ] Commit "Implement the console pages from the design".

## Task 8: Ship the console in the chart, with docs and a self-check

Files:

- `charts/kuvryn-sync/values.yaml`: `console.enabled: false`, `console.oidc.issuerURL`,
  `console.oidc.clientID`, `console.oidc.clientSecret.secretName` and
  `.key`, `console.redirectURL`, `console.usernameClaim`,
  `console.groupsClaim`, `console.usernamePrefix`, `console.groupsPrefix`,
  `console.sessionKey.secretName` and `.key`, `console.clusterName`,
  `console.connectors`, `console.docsURL`, `console.statusURL`,
  `console.ingress.{enabled, className, host, tls}`, and
  `console.resources`.
- `charts/kuvryn-sync/templates/console.yaml`: a Deployment
  (`<release>-kuvryn-sync-console`, args `console` with flags, Secrets
  mounted read-only at `/etc/ksync/oidc` and `/etc/ksync/session`,
  runAsNonRoot, readOnlyRootFilesystem), a ServiceAccount, a Service on port
  80 → 8080, an optional Ingress, and a ClusterRole plus ClusterRoleBinding
  that grant `impersonate` on `users` and `groups` only.
- `internal/console/selfcheck.go`: at startup, it creates two
  `SelfSubjectAccessReview`s for impersonate on users and groups, logs the
  result, and sets the `/healthz` `"impersonation":"granted"` or `"missing"`
  field.
- `docs/console.md`: the Dex setup end to end:
  - a Dex static client with the redirect URL;
  - GitHub and GitLab connectors, and the local password connector;
  - the chart values;
  - viewer RBAC examples: a ClusterRole `kuvryn-sync-viewer`, with get and
    list on `sync.kuvryn.io` resources and the managed kinds, bound to a
    group;
  - troubleshooting for claims, `system:` identities and namespace-scoped
    viewers.
- `docs/index.md` and `README.md`: add the console, with a screenshot taken
  from `make test-ui`.
- `internal/controller/rbac_manifest_test.go`: the new tests.

Interfaces: consumes the `ksync console` flags from Task 1. Produces the
chart values listed above, and the console names
`<release>-kuvryn-sync-console`.

- [ ] Write in `internal/controller/rbac_manifest_test.go`:
  ```go
  func TestHelmChartRendersASeparateConsole(t *testing.T) {
  	out := helmTemplate(t, "--set", "console.enabled=true", "--set", "console.oidc.issuerURL=https://dex.example", "--set", "console.oidc.clientID=ksync")
  	if !strings.Contains(out, "name: kuvryn-sync-kuvryn-sync-console") {
  		t.Fatal("no console Deployment")
  	}
  	if strings.Count(out, "serviceAccountName: kuvryn-sync-kuvryn-sync-console") != 1 {
  		t.Fatal("the console does not run as its own ServiceAccount")
  	}
  }

  func TestConsoleClusterRoleOnlyImpersonates(t *testing.T) {
  	role := clusterRoleNamed(t, helmTemplate(t, "--set", "console.enabled=true", "--set", "console.oidc.issuerURL=https://dex.example", "--set", "console.oidc.clientID=ksync"), "kuvryn-sync-kuvryn-sync-console")
  	for _, rule := range role.Rules {
  		if !slices.Equal(rule.Verbs, []string{"impersonate"}) || !slices.Equal(rule.APIGroups, []string{""}) {
  			t.Fatalf("console rule grants more than impersonate: %+v", rule)
  		}
  		for _, r := range rule.Resources {
  			if r != "users" && r != "groups" {
  				t.Fatalf("console may impersonate %s", r)
  			}
  		}
  	}
  }
  ```
  Run `go test ./internal/controller/ -run 'Console'`, and expect it to FAIL
  with "no console Deployment".
- [ ] Implement the chart templates, `selfcheck.go` and the docs. Run
      `helm lint charts/kuvryn-sync --set console.enabled=true --set console.oidc.issuerURL=https://dex.example --set console.oidc.clientID=ksync`,
      and expect 0 failures.
- [ ] Run `go test ./internal/controller/ ./internal/console/ && make test && make lint`,
      and expect PASS.
- [ ] Commit "Ship the console in the chart with Dex docs and a self-check".

## Task 9: Review, merge, and hand over to the release

Files:

- The console branch as a whole.
- `.github/workflows/test-ui.yml`: a new workflow running `make test-ui` on
  `arc-azrtydxb-amd64` with Playwright's Chromium
  (`npx --prefix web playwright install --with-deps chromium`). Fork PRs run
  it on `ubuntu-latest`.

Interfaces: consumes Tasks 1–8. Produces the merged console on `main`, ready
for the kuvryn-sync-rebrand Task 11 release (v0.4.0).

- [ ] Add `test-ui.yml`, then push the branch. Expect Tests, Lint, E2E,
      Image and UI to be green; the UI run fails if any Playwright test fails.
- [ ] Run the fresh-context pre-PR review (REVIEW.md rubric plus the
      simplify lens) with a security focus on auth, sessions and impersonation.
      Fix the Critical and Important findings, then open the PR "Add the
      read-only Kuvryn Sync console".
- [ ] Merge once every check is green and every review thread is answered.
      Then run the rebrand plan's Task 11 to release v0.4.0.
