---
title: Security model
nav_order: 9
---

# Security model

Kuvryn Sync is designed to keep sensitive material out of public operational
surfaces while still giving operators useful plans, events, and diagnostics.

## Secret handling

- Git credentials are referenced through Kubernetes Secrets. Kuvryn Sync only uses
  Secrets labelled `sync.kuvryn.io/git-credentials: "true"`, because whoever writes a
  Repository chooses both the Git URL and the Secret; without the label, any
  Secret in the namespace could be sent to an arbitrary Git server.
- Secret values are not copied into Repository, Application, or Revision specs.
- Plan, status, log, Event, metrics, trace, and CLI paths use centralized
  redaction. It removes `password=`-style assignments, bearer tokens, and
  credentials embedded in URLs (`https://user:token@host` becomes
  `https://REDACTED@host`).
- Desired Secret manifests are treated as sensitive even when rendered from Git.
- Diagnosis reads Secrets, ConfigMaps, and ServiceAccounts as metadata only,
  so their data never reaches `status.diagnosis`; container messages it quotes
  are redacted and cut to 512 characters.

## Web console

The optional [web console](console.md) runs as its own Deployment and
ServiceAccount. People sign in with a Kubernetes bearer token or, when it is
configured, OIDC.

A token session can read exactly what its token's RBAC allows, and nothing
else:

- Every read carries the session's own bearer token and nothing of the
  console's: its client starts from the API server's address and CA only, so
  the console ServiceAccount's token, client certificates, basic auth,
  impersonation settings and credential plugins never reach the request, and
  the transport refuses any `Impersonate-*` header or other `Authorization`
  value before the request is sent. The audit log records the token's user.
- A token session cannot write, read Secrets, or open logs, exec or other
  subresources, whatever the token itself may do, because of the client
  rules below.
- The token is validated with a SelfSubjectReview sent with it (Kubernetes
  1.28 or later). `system:anonymous`, the `system:unauthenticated` group and
  `system:` users other than ServiceAccounts are refused, as are tokens over
  16 KiB and JWTs past their `exp`.
- The token is kept only in the encrypted session cookie, never logged,
  never returned by an API and never put in a URL. The session ends at the
  token's `exp` or 8 hours after sign-in, whichever is first, and a 401 from
  the API server ends it early. Signing out does not revoke the token; delete
  its ServiceAccount, or the object it is bound to, for that.
- A token-only console's ServiceAccount has no permissions: the chart
  renders no ClusterRole or binding for it.

With OIDC, the console's ServiceAccount may `impersonate` `users` and
`groups`. That permission is effectively cluster-admin: it covers any user
and group, `system:masters` included, and the console's refusal of `system:`
identities is enforced only inside the console process. Whoever holds the
console ServiceAccount's token can act as cluster-admin, so restrict the
release namespace (pod exec, pod creation and Secret reads) to cluster
admins, and limit the role with `console.impersonation.users` and
`console.impersonation.groups`, which render `resourceNames` on the
impersonate rules.

- Every OIDC read impersonates the signed-in user and their groups, so
  Kubernetes RBAC decides what each person sees, and the API server's audit
  log records the reads under their name. Usernames and groups starting with
  `system:` are refused.
- Its client sends only GET requests for API discovery and for lists and
  gets of resources. It refuses everything else before it is sent: Secrets
  however the path is spelled, subresources such as proxies, exec, attach,
  port-forward and logs, watches, and connection upgrades. Secrets appear
  only as names recorded in a plan.
- OIDC sign-in is the code flow with PKCE (S256), state and nonce, and the ID
  token is verified against the issuer's keys. The session is an AES-256-GCM
  encrypted, HttpOnly, Secure, SameSite=Lax cookie that expires with the ID
  token; no refresh token is stored.
- Pages are served with a strict Content-Security-Policy:
  `default-src 'self'; img-src 'self' data:; style-src 'self'; script-src 'self'; frame-ancestors 'none'; form-action 'self'; base-uri 'none'`.
- Cross-site POSTs, such as a forged sign-out or a forged token sign-in, are
  refused with 403, judged by the browser's `Sec-Fetch-Site` or `Origin`
  header.
- Every message it returns goes through the same redaction as the CLI.

## Server-Side Apply ownership

Kuvryn Sync mutates live objects with Server-Side Apply. The default conflict policy
is `fail`, which prevents Kuvryn Sync from taking fields owned by another manager.
`conflictPolicy: adopt` takes them over deliberately, for migrations: the plan
lists every field and its previous manager, and manual approval, when
enabled, applies to the takeover like any other change.

## RBAC and service account impersonation

Kuvryn Sync reads, applies, and prunes an Application's resources as a service
account in the Application's namespace, not as the controller. Kubernetes RBAC
therefore decides what each Application may change: an Application cannot
create a ClusterRoleBinding, or touch another team's namespace, unless its
service account could do so itself.

The service account is `spec.serviceAccountName`, or the manager's
`--default-service-account` (Helm value `defaultServiceAccount`) when the
Application sets none. The default is a name, looked up in each Application's
namespace. When neither is set, Kuvryn Sync refuses the Application with a
`ServiceAccountRequired` condition and neither reads nor changes its managed
resources.

An RBAC denial while reading, applying, or pruning fails the Revision with
reason `Forbidden`. Kinds the service account may not list are left out of
pruning and reported with a `PruneInventoryIncomplete` Warning Event. The
service account is part of the Revision identity, so switching an Application
to an account with the right permissions starts a fresh Revision.

Diagnosis also reads as the Application's service account: the Pods,
ReplicaSets, and EndpointSlices below managed resources and the objects they
refer to. It never widens what Kuvryn Sync can see, and a read the account may not
make only makes the diagnosis shallower. See
[Diagnosis permissions](operations.md#diagnosis-permissions). `ksync graph`
reads with the caller's own kubeconfig credentials instead.

An Application may name any service account in its own namespace. Creating
Applications in a namespace is therefore as powerful as the most privileged
service account there: grant it only to people who could already act as those
accounts.

A typical tenant grant binds the built-in `admin` ClusterRole in the
destination namespace only:

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: payments-deployer
  namespace: default
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: payments-deployer
  namespace: payments
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: admin
subjects:
  - kind: ServiceAccount
    name: payments-deployer
    namespace: default
```

The controller's own role cannot change managed resources at all. It may
manage Kuvryn Sync's CRDs, record Events, impersonate service accounts, list and
watch the metadata of the kinds it watches for drift, and read Secrets. Review
`config/rbac/role.yaml`; the Helm chart role is kept identical to it by a test.

The Secret grant is cluster-wide `get`, `list` and `watch`. RBAC cannot limit a
grant to metadata or to labelled Secrets, so the controller's service account
can read every Secret in the cluster; protect it accordingly. What the manager
does with that grant is narrower:

- It watches Secrets metadata-only, to notice drift on Secrets it manages. Only
  that metadata is cached; Secret contents are never cached.
- It reads a Secret's contents with an uncached `get`, by the name an object in
  the same namespace references, and only for these purposes:
  - Git credentials for fetching and image write-back
    (`spec.git.auth.secretRef`, `spec.imageUpdate.secretRef`) must be labelled
    `sync.kuvryn.io/git-credentials: "true"`.
  - Registry credentials for ImagePolicy scans and Helm chart pulls
    (`spec.secretRef` of an ImagePolicy, `render.helm.chart.secretRef`) must be
    labelled `sync.kuvryn.io/registry-credentials: "true"`.
  - age keys for decryption (`spec.decryption.secretRef`) must be labelled
    `sync.kuvryn.io/decryption-key: "true"`.
  - Webhook receiver tokens (`spec.webhook.secretRef` of a Repository or
    ImagePolicy) and NotificationSink Secrets (`spec.secretRef`) need no label.
    They are only ever compared against incoming requests or used to reach the
    sink's own URL, and are read from the referencing object's namespace.
- Helm `valuesFrom` Secrets are read as the Application's service account, not
  as the controller.

The labels exist because whoever writes a Repository, ImagePolicy or
Application chooses both the destination and the Secret. Without them, any
Secret in the namespace could be sent to a server of the author's choosing.

## Supply chain

Published controller images are built by GitHub Actions and pushed to GHCR:

```text
ghcr.io/azrtydxb/kuvryn-sync:<tag>
```

For production, pin immutable tags or digests and use your cluster's image
policy controls.

## Network access

Besides the Kubernetes API, the controller makes outbound connections to:

- Git remotes of Repositories, to fetch.
- Git remotes of Repositories with `spec.imageUpdate`, to push image
  write-back commits.
- Helm chart repositories (`https://`) and OCI registries (`oci://`) named in
  an Application's `render.helm.chart`, to pull the pinned chart.
- Container registries of ImagePolicies, to list tags.
- NotificationSink endpoints, which must be `https` URLs.
- The OTLP trace collector, only when `OTEL_EXPORTER_OTLP_ENDPOINT` or
  `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` is set. Spans carry redacted error
  messages and no Application names.

It accepts inbound connections on the admission webhook port (9443), the
metrics port (8443), the health probe port (8081), and, only when the manager
runs with `--webhook-receiver-bind-address` (Helm value
`webhookReceiver.enabled`), the push webhook receiver. The receiver takes
`POST /hooks/<namespace>/<repository>` and
`POST /hooks/imagepolicies/<namespace>/<name>`, answers only for objects that
set `spec.webhook`, and authenticates each request against that object's token
Secret.

Git, Kustomize, and Helm run in process; the controller image contains no git,
kustomize, or helm binary and runs no subprocesses.

Rendering reads the checked-out commit and, for `render.helm.chart`, the pulled
chart. Checkouts follow every symlink, including chains of links, and refuse
the commit if any resolves outside the checkout; files are created
exclusively, so nothing is written through a link. Kustomize builds against an
in-memory copy of the checkout so bases outside it do not exist, and every
remote reference a kustomization names (URLs, Git remotes, and `?ref=`
sources, in resources, bases, components, generators, and patches) is refused
before Kustomize can fetch it. Helm values files must lie inside the
checkout, and charts in the checkout must vendor their dependencies. Repository
URLs must use `https`, `http`, `ssh`, or `git`; filesystem paths are rejected.
SSH remotes require a `known_hosts` entry in the credentials Secret, so an
unknown or changed host key is never trusted.

## Responsible disclosure

Please report security issues privately to the repository owner rather than
opening a public issue with exploit details.
