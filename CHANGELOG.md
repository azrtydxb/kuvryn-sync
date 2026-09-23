# Changelog

## Unreleased

- **Breaking:** Solder now reads, applies, and prunes each Application's
  resources by impersonating a service account, so tenant RBAC decides what an
  Application may change. Set `spec.serviceAccountName`, or start the manager
  with `--default-service-account` (Helm value `defaultServiceAccount`);
  Applications with neither are refused with `ServiceAccountRequired`.
- **Breaking:** Git credential Secrets must be labelled
  `solder.io/git-credentials: "true"`. Previously a Repository author could
  point `secretRef` at any Secret in the namespace and a Git URL they control,
  and Solder would send that Secret to it.
- RBAC denials while reading, applying, or pruning fail the Revision with
  reason `Forbidden`. Kinds the service account may not list are skipped by
  pruning and reported with a `PruneInventoryIncomplete` Warning Event.
- The service account is part of the Revision identity, so switching accounts
  starts a fresh Revision instead of reusing one blocked by retry limits.
- `--default-service-account` is validated at startup.
- SOPS decryption with age keys: `spec.decryption` points at a Secret
  labelled `solder.io/decryption-key: "true"`; the `yaml` and `kustomize`
  renderers decrypt SOPS files in memory with MAC verification, and encrypted
  files without `spec.decryption` fail instead of being applied as ciphertext.
  Uses the official sops library, which grows the manager binary by about 45 MB.
- `spec.dependsOn` orders Applications in a namespace: a dependent plans but
  applies only once its dependencies are Healthy at their desired revision,
  with the `DependenciesReady` condition explaining any wait or cycle.
- Lifecycle notifications: the new `NotificationSink` API (HMAC-signed
  webhook or Slack, destination in a Secret) and `spec.notifications` on
  Applications send `AwaitingApproval` (with the approve command), `Healthy`,
  `Failed`, and `RolledBack` once per transition. Delivery is queued, retried,
  and never blocks reconciliation; misconfiguration shows as the
  `NotificationsReady` condition.
- Fixed: the labels and annotation Solder stamps on applied objects were
  planned as drift on the next reconcile, so every Application re-applied
  forever and manual approvals went stale immediately after applying.
- Manual approvals are attributable and bound to the plan: a mutating
  admission webhook records the authenticated approver, time, and plan digest
  whenever `solder.io/approved-revision` changes, and reverts hand-edited
  records. Revisions carry `status.plan.digest`; Solder applies only when the
  approved digest still matches, otherwise the Revision returns to
  AwaitingApproval with an `ApprovalStale` Event. Applied Revisions record
  `status.approval`. **Breaking:** an `approved-revision` annotation without the
  webhook's record no longer applies anything.
- `solder history -o json` exports the audit trail; `solder approve` aliases
  `solder sync`.
- Git, Kustomize, and Helm now run in process (go-git, kustomize/api, the
  Helm v4 SDK). Fixed: the controller image never shipped kustomize or helm,
  so `kustomize` and `helm` Applications could not render in it. The image no
  longer contains git either.
- Rendering is confined to the checkout: symlinks pointing outside it are
  refused, Kustomize cannot load bases outside it, and Helm values files must
  lie inside it (previously `../` values paths could read controller files).
- **Breaking:** Repository URLs must be `https`, `http`, `ssh`, or `git`;
  filesystem paths are rejected. SSH remotes require `known_hosts` in the
  credentials Secret. Kustomize remote bases are not fetched, and Helm chart
  dependencies must be vendored into `charts/`.
- Helm charts render with `.Release.Namespace` set to the destination
  namespace instead of `default`.
- New cluster-scoped `HealthCheck` API: ordered CEL rules decide the health of
  a kind, falling back to kstatus when none matches. Rules run with a cost
  limit and fail closed as Progressing.
- **Breaking (install):** a validating webhook rejects HealthChecks whose
  expressions do not compile to a bool. cert-manager is now required: the raw
  manifests and the Helm chart create an Issuer and Certificate for the
  webhook. Install the new `healthchecks.solder.io` CRD with the others.
- Health for kinds without a dedicated rule follows kstatus conventions
  (`observedGeneration`, `Stalled`, `Reconciling`, `Ready`); objects without a
  status are Healthy. Jobs are Healthy when complete and Degraded when failed.
  Previously every other kind was `Unknown` and held the rollout until
  `health.timeout` triggered a failure or rollback; `Unknown` no longer holds a
  rollout.
- `status.managedKinds` records the kinds an Application manages. Pruning now
  covers objects of any kind, not just ConfigMaps, Secrets, Services,
  Deployments, StatefulSets, and DaemonSets.
- Drift on any managed kind is detected: immediately through a metadata-only
  watch when the controller may list/watch the kind, otherwise every
  `--drift-resync-interval` (Helm value `driftResyncInterval`, default 5m).
- Fixed: once retry limits were reached, every reconcile replaced the
  Revision's failure with `RetryBlocked`, hiding the real cause. The Revision
  now keeps its original failure and the Application's Ready condition reports
  `RetryBlocked` together with it.
- **Breaking:** Applications discovered from `.solder.yaml` run as the new
  Repository field `spec.applicationServiceAccountName` and may not name any
  other service account; when the Repository sets none, they may not set one
  and use the controller default. Git write access can no longer choose which
  service account Solder acts as.
- Rendered objects without a namespace are placed in `destination.namespace`
  only when the cluster (or a CustomResourceDefinition rendered alongside
  them) says their kind is namespaced. Previously any cluster-scoped kind
  outside a short built-in list, such as StorageClass, was given the
  destination namespace. Kinds the cluster does not know fail with a
  retryable `ValidationFailure`.
- The controller ClusterRole no longer grants any write access to managed
  resources, RBAC objects, or CRDs; it keeps list/watch on the kinds it
  watches for drift. Those watches are metadata-only and Secrets are read
  uncached, so Secret contents are never held in controller memory. The Helm
  chart role now matches the generated role (it previously lacked the
  permissions the watches need).
- `status.serviceAccountName` and the `solder apps` output show the
  impersonated service account.
- The controller role gains `impersonate` on service accounts.

## 0.1.11

- Added Repository `spec.applicationConfigPaths` for monorepo and moved `.solder.yaml` discovery.
- Supported multiple Applications per `.solder.yaml` file through the `applications:` list.
- Documented config-path validation, duplicate Application-name rejection, discovery annotations, and pruning behavior.
- Updated Helm/default install examples to the `v0.1.11` image tag.

## 0.1.10

- Added product-path E2E coverage for Repository, Application, Revision, and applied workload reconciliation.
- Made E2E setup idempotent and included E2E build-tag linting.
- Reconciled stale Procoder planning signals after MVP closure.
- Fixed local Procoder CLI version mismatch so finish review can run `procoder review`.
