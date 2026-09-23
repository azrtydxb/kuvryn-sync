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
