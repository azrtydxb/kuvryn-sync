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
