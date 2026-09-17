# Changelog

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
