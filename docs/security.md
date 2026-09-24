---
title: Security model
nav_order: 9
---

# Security model

Solder is designed to keep sensitive material out of public operational
surfaces while still giving operators useful plans, events, and diagnostics.

## Secret handling

- Git credentials are referenced through Kubernetes Secrets. Solder only uses
  Secrets labelled `solder.io/git-credentials: "true"`, because whoever writes a
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

## Server-Side Apply ownership

Solder mutates live objects with Server-Side Apply. The default conflict policy
is `fail`, which prevents Solder from taking fields owned by another manager.
`conflictPolicy: adopt` takes them over deliberately, for migrations: the plan
lists every field and its previous manager, and manual approval, when
enabled, applies to the takeover like any other change.

## RBAC and service account impersonation

Solder reads, applies, and prunes an Application's resources as a service
account in the Application's namespace, not as the controller. Kubernetes RBAC
therefore decides what each Application may change: an Application cannot
create a ClusterRoleBinding, or touch another team's namespace, unless its
service account could do so itself.

The service account is `spec.serviceAccountName`, or the manager's
`--default-service-account` (Helm value `defaultServiceAccount`) when the
Application sets none. The default is a name, looked up in each Application's
namespace. When neither is set, Solder refuses the Application with a
`ServiceAccountRequired` condition and neither reads nor changes its managed
resources.

An RBAC denial while reading, applying, or pruning fails the Revision with
reason `Forbidden`. Kinds the service account may not list are left out of
pruning and reported with a `PruneInventoryIncomplete` Warning Event. The
service account is part of the Revision identity, so switching an Application
to an account with the right permissions starts a fresh Revision.

Diagnosis also reads as the Application's service account: the Pods,
ReplicaSets, and EndpointSlices below managed resources and the objects they
refer to. It never widens what Solder can see, and a read the account may not
make only makes the diagnosis shallower. See
[Diagnosis permissions](operations.md#diagnosis-permissions). `solder graph`
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
manage Solder's CRDs, record Events, impersonate service accounts, list and
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
    `solder.io/git-credentials: "true"`.
  - Registry credentials for ImagePolicy scans and Helm chart pulls
    (`spec.secretRef` of an ImagePolicy, `render.helm.chart.secretRef`) must be
    labelled `solder.io/registry-credentials: "true"`.
  - age keys for decryption (`spec.decryption.secretRef`) must be labelled
    `solder.io/decryption-key: "true"`.
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
ghcr.io/azrtydxb/solder:<tag>
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
