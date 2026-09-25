# kuvryn-sync-console

Status: complete

## Problem

Kuvryn Sync can be inspected today only with `kubectl` and the `ksync` CLI.
That's fine for operators, but not for the people who need to see state
without touching it:

- application teams asking "why is my app Degraded";
- approvers checking what a plan will change;
- auditors reading the revision history;
- on-call staff scanning which apps are drifted or awaiting approval.

Each of them needs a kubeconfig, the CLI, and knowledge of which object holds
which field. The diagnosis chains, plans and revision audit records already
exist in status. They need a read-only view that anyone can open in a
browser, that shows exactly what their Kubernetes RBAC already lets them see,
and that never becomes a second path for changing the cluster.

## Users

- **Application teams:** see their Applications' sync and health, the
  diagnosis chain to the root cause, the resources, and the history, without
  kubectl.
- **Approvers:** read the plan and its digest before approving with the CLI;
  the console shows the exact `ksync` command.
- **Auditors:** read Revisions: who approved what, which plan digest, and
  when.
- **Platform operators:** install and configure the console (OIDC, ingress,
  RBAC) and see Repositories and Image policies across teams.
- **Cluster admins:** decide what each person sees through ordinary RBAC
  bindings; the console adds no permission model of its own.

## In scope

- [S-1] A `ksync console` subcommand serves the console: the HTTP server, the
  JSON API and the embedded SPA. It runs as its own Deployment with its own
  ServiceAccount, separate from the controller. The Helm chart installs it
  optionally, off by default.
- [S-2] OIDC sign-in using the authorization code flow with PKCE, against any
  OIDC issuer (Dex in the design). The login page offers:
  - the SSO button;
  - optional GitHub, GitLab and email/password buttons, which pass Dex's
    `connector_id`;
  - sign-out.
    Sessions live in an encrypted, HttpOnly, Secure, SameSite=Lax cookie.
- [S-3] Every cluster read is made through a client that impersonates the
  signed-in user's username and groups, taken from configurable ID-token
  claims. The console can only send get, list and watch. Usernames and groups
  starting with `system:` are refused.
- [S-4] A read-only JSON API (the endpoints are listed under Interfaces),
  scoped to the Kuvryn Sync CRDs and to the Application's managed objects for
  the Resources tab. Secret objects are never read or returned.
- [S-5] Applications list page: stat cards (applications, healthy, not
  synced, awaiting approval), a Degraded alert with a "View diagnosis" link,
  and a table with name, destination, repository, commit, sync, health and
  last reconcile.
- [S-6] Application detail with tabs:
  - Overview: source and sync-policy properties, and conditions;
  - Diagnosis: status.diagnosis causal chains, with the empty state;
  - Plan: the newest Revision's summary, digest, phase and resources with
    field changes;
  - History: Revisions with phase, plan, approver and attempts;
  - Resources: managed objects with sync and health.
    The Diagnosis and Plan tabs show the exact `ksync` commands for the
    equivalent CLI view or approval.
- [S-7] A Repositories page (URL, ref, observed commit, state, app count,
  poll interval, webhook, last fetch), a Revisions page (every Revision,
  newest first) and an Image policies page (image, rule, latest tag and
  digest, write target, last scan).
- [S-8] The UI implements the claude.ai/design Login and Console designs with
  the Azrty design system in the Build pillar. It covers dark and light
  themes, the split and centred login layouts, the Kuvryn Sync emblem and
  lockup, and narrow widths. It is built with React, Vite and TypeScript and
  embedded in the Go binary.
- [S-9] Data refreshes in the page without a reload: the UI re-fetches the
  JSON API every 10 seconds, and the top bar shows the last refresh time.
  There are no server-side watches per user.
- [S-10] The chart (`charts/solder/values.yaml`, renamed to charts/kuvryn-sync
  by the rebrand) and the docs (`docs/operations.md`, plus a new console page)
  cover:
  - console values (OIDC issuer, client ID and Secret reference, redirect
    URL, claims, session-key Secret, cluster display name, links);
  - Service and optional Ingress;
  - a ClusterRole that grants the console only `impersonate` on users and
    groups;
  - documentation for setup with Dex, and for which RBAC a viewer needs.

## Out of scope

- Any write action from the UI: approve, sync, roll back, suspend, delete.
  The UI shows the CLI command instead.
- A user database, password storage or user management. Passwords go through
  Dex's own connectors.
- Multi-cluster views. One console serves one cluster, and the display name
  is configured.
- Showing Secret contents, or Secret objects at all.
- Log streaming, Pod exec and arbitrary resource browsing beyond the
  Application's managed objects.
- Its own audit log UI. Kubernetes audit logs record the impersonated reads.

## Constraints

- Security:
  - the console ServiceAccount's only permission is `impersonate` on `users`
    and `groups`, and never on `serviceaccounts` or `system:` groups;
  - every API call is impersonated, and the console never reads as itself;
  - the only verbs are get, list and watch;
  - ID tokens are verified (issuer, audience, expiry, signature via JWKS);
  - PKCE, state and nonce are required;
  - there is a strict Content-Security-Policy with no inline script;
  - all output is redacted with `internal/redact`.
- The console never reuses the controller's client, cache or credentials.
- The SPA is embedded (embed.FS), so no Node is needed at runtime.
  `web/dist` is not committed: the Dockerfile and CI build it with Node, and a
  plain `go build` without a built UI embeds a placeholder page that explains
  how to build it.
- It works behind an Ingress with TLS. Plain HTTP is allowed only with an
  explicit `--insecure-cookies` flag for local development.
- The pages follow the design system's content rules: identifiers are
  monospace and verbatim, unknown values are shown as "—", there are no
  emoji, and headings are in sentence case.

## Interfaces

- **Command:** `ksync console` with these flags:
  - `--listen`;
  - `--oidc-issuer-url`, `--oidc-client-id`, `--oidc-client-secret-file`;
  - `--redirect-url`;
  - `--username-claim` (default `email`), `--groups-claim` (default
    `groups`), `--username-prefix` and `--groups-prefix` (both empty by default, so RBAC
    bindings name users and groups as the IdP sends them);
  - `--session-key-file`;
  - `--cluster-name`;
  - `--connectors` (for example `github,gitlab,local`);
  - `--docs-url`, `--status-url`;
  - `--insecure-cookies`.
- **HTTP routes:**
  - `GET /login`, `GET /auth/start?connector=`, `GET /auth/callback`,
    `POST /logout`;
  - `GET /api/me` and `GET /api/namespaces` (for the namespace picker);
  - `GET /api/applications` and `GET /api/applications/{ns}/{name}`;
  - `GET /api/applications/{ns}/{name}/revisions`;
  - `GET /api/applications/{ns}/{name}/resources`;
  - `GET /api/repositories`, `GET /api/revisions`, `GET /api/imagepolicies`;
  - `GET /healthz`;
  - every other path serves the SPA.
- **JSON:** the API returns Kubernetes objects filtered to the fields the
  pages show. Managed-field and annotation noise is dropped.
- **UI routes:** `/login`, `/apps`, `/apps/{ns}/{name}/{tab}`, `/repositories`,
  `/revisions`, `/imagepolicies`.

## Data

- **Stored:** nothing server-side. The session cookie holds the encrypted
  username, groups and expiry. A session ends when the ID token expires; no
  refresh tokens are stored, and the user signs in again.
- **Configuration:** from flags and Secrets. The OIDC client secret and the
  session key are mounted from Kubernetes Secrets.
- **Browser:** the theme choice is kept in localStorage.

## Edge cases

- **Namespace-scoped viewers:** a user who may list Applications in only some
  namespaces gets Forbidden on cluster-wide list. The console tries
  cluster-wide first. On Forbidden, it shows a namespace picker filled from
  the namespaces the user may list, with typed entry as the fallback when
  even that is forbidden. The choice is remembered in localStorage per
  browser.
- **Groups claim:** an ID token with no groups claim means username only; a
  token with 500+ groups must stay bounded in the cookie.
- **Token expiry:** a token expiring mid-session sends the user back to login
  on the next API call (401 → redirect), not to a broken page.
- **Missing status fields:** an Application with no status yet (just created)
  shows "—" and Unknown, not errors.
- **Diagnosis:** an empty diagnosis shows the empty state. A cause pointing
  at an unreadable object shows "unreadable" as recorded.
- **Revision lookup:** the plan tab finds the newest Revision by
  `sync.kuvryn.io/application` and `spec.applicationRef`. With none, it shows
  "No plan yet".
- **Impersonated reads:** a user without list on a managed kind sees a
  "not visible with your permissions" row in Resources, not an error page.
- **Session key rotation:** rotating the key logs everyone out cleanly.

## Failure modes

- **OIDC issuer unreachable at startup:** start anyway, report not ready on
  `/healthz`, and retry discovery. At login, show the design's "Sign-in
  failed" alert.
- **API server slow or unavailable:** API calls time out (bounded, for
  example 10s) and the UI shows a non-blocking error banner. The last good
  data stays visible with its refresh time.
- **Impersonation not granted:** every read fails with Forbidden. The console
  logs a clear startup self-check result, running a SubjectAccessReview for
  impersonate, and `/healthz` reports it.
- **Wrong claims configuration** (for example, no email claim): the login
  fails with a message naming the missing claim, not a 500.

## Acceptance criteria

- [ ] [S-1] `TestHelmChartRendersASeparateConsole` renders `charts/kuvryn-sync`
      with `console.enabled=true`. It fails if there is no console Deployment,
      or if the console shares the controller's ServiceAccount.
- [ ] [S-2] `TestOIDCLoginFlow`, run against an httptest OIDC issuer, fails if: - the code flow with PKCE does not set an HttpOnly, Secure, SameSite=Lax
      cookie; - a callback with a wrong state or nonce is accepted; - an unverifiable ID token is accepted.
- [ ] [S-3] `TestConsoleFollowsUserRBAC` (envtest with RBAC) fails if a user
      bound to view Applications only in namespace `a` sees anything from
      namespace `b`, if a read is not impersonated, or if a `system:`
      username or group is admitted.
- [ ] [S-3] `TestConsoleClientIsReadOnly` fails if create, update, patch or
      delete reaches the recording transport.
- [ ] [S-4] `TestConsoleNeverReturnsSecrets` calls every API endpoint with a
      Secret present, and fails if any response contains a Secret object or
      Secret data.
- [ ] [S-5] [S-6] [S-7] `TestConsolePages` (`make test-ui`, which runs `web/e2e/console.spec.ts`) renders
      the Applications list, every Application tab, Repositories, Revisions
      and Image policies against a seeded envtest cluster. It fails if a
      designed element is missing: the stat cards, tables, tabs, diagnosis
      chain or CLI hints.
- [ ] [S-8] `TestLoginPage` (`make test-ui`, which runs `web/e2e/login.spec.ts`) renders the login page in the dark and
      light themes and the split and centered layouts, and runs axe. It fails
      if axe reports any serious violation, including contrast.
- [ ] [S-9] `TestLiveRefresh` (`make test-ui`, which runs `web/e2e/refresh.spec.ts`) changes an Application's status in the
      cluster, and fails if the open page does not show the change within one
      10-second refresh without a reload.
- [ ] [S-10] `TestConsoleClusterRoleOnlyImpersonates` fails if the console
      ClusterRole in `charts/kuvryn-sync` grants anything but `impersonate` on
      `users` and `groups`. `docs/console.md` includes an end-to-end Dex setup.

## Open questions

<!-- All resolved with the maintainer on 2026-09-25; decisions recorded above. -->
