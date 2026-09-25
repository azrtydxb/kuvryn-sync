# Rebrand 2: Move every CRD to the sync.kuvryn.io API group

Status: closed 2026-09-25
Created: 2026-09-25
Plan: .procoder/plans/kuvryn-sync-rebrand.md

## Description

groupName, rbac and webhook markers, PROJECT domain/group, regenerated CRDs, and chart rules.

## Acceptance criteria

- [x] `TestCRDsUseTheKuvrynSyncGroup` fails first, then passes
- [x] `make manifests generate` leaves no diff and `TestHelmChartRoleMatchesGeneratedRole` passes
- [x] `procoder check` has no blocking findings for the resulting change.

## Evidence

- `go test ./internal/brand/ -run TestCRDsUseTheKuvrynSyncGroup` before: FAIL, "missing CRD ../../config/crd/bases/sync.kuvryn.io_applications.yaml"; after regeneration: `ok`.
- `make manifests generate` after staging: `git status` showed no unstaged change; `go test ./internal/controller/ -run TestHelmChartRoleMatchesGeneratedRole -v`: PASS.
- `make test` exit 0 (first run caught `TestControllerRoleOnlyWritesSolderObjects` still expecting the `solder.io/` prefix; fixed); `make lint`: "0 issues."; `go vet -tags e2e ./test/e2e/` clean.
- `procoder check` flagged nine scaffolded `config/rbac/*_role.yaml` files as unformatted (pre-existing indentation); formatted with `procoder format`, then "0 unformatted ... (0 blocking)"; `bin/kustomize build config/default` still builds.
