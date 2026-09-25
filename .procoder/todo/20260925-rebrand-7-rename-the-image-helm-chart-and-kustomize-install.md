# Rebrand 7: Rename the image, Helm chart, and kustomize install

Status: closed 2026-09-25
Created: 2026-09-25
Plan: .procoder/plans/kuvryn-sync-rebrand.md

## Description

charts/kuvryn-sync, namespace kuvryn-sync-system, namePrefix kuvryn-sync-, and image ghcr.io/azrtydxb/kuvryn-sync.

## Acceptance criteria

- [x] `TestHelmChartUsesKuvrynSyncNames` fails first, then passes
- [x] `helm lint charts/kuvryn-sync` and `make test` pass
- [x] `procoder check` has no blocking findings for the resulting change.

## Evidence

- `go test ./internal/controller/ -run TestHelmChartUsesKuvrynSyncNames` before: FAIL, "helm template: resolve render path charts/kuvryn-sync: ... no such file or directory"; after `git mv charts/solder charts/kuvryn-sync` and the renames: PASS, with `TestHelmChartRoleMatchesGeneratedRole` and `TestHelmChartServicesSelectTheirOwnRelease` also passing on the new path.
- `helm lint charts/kuvryn-sync`: "1 chart(s) linted, 0 chart(s) failed"; `bin/kustomize build config/default | grep -c kuvryn-sync-system`: 18.
- `make test` exit 0; `make lint`: "0 issues."; `make manifests generate` after staging left no diff; `make build-installer IMG=ghcr.io/azrtydxb/kuvryn-sync:v0.4.0` writes `image: ghcr.io/azrtydxb/kuvryn-sync:v0.4.0` and `- /ksync` (the 15 remaining old-name lines are API doc prose, rewritten in Task 9).
- `procoder check` first flagged the moved chart templates (the `.prettierignore` glob still named `charts/solder`; fixed, since prettier turns `{{ include }}` into `{ { include } }`) and five plain YAML files plus `dist/install.yaml` (formatted with `procoder format`; `bin/kustomize build config/default` and a YAML parse of the installer still succeed); then "0 unformatted ... (0 blocking)".
