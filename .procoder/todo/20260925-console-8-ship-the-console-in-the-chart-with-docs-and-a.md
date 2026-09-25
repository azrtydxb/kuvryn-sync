# Console 8: Ship the console in the chart with docs and a self-check

Status: closed 2026-09-25
Created: 2026-09-25
Plan: .procoder/plans/kuvryn-sync-console.md

## Description

Chart values and templates, an impersonate-only ClusterRole, the SSAR self-check, docs/console.md with Dex, and the README.

## Acceptance criteria

- [x] `TestHelmChartRendersASeparateConsole` and `TestConsoleClusterRoleOnlyImpersonates` fail first, then pass
- [x] `helm lint` with the console enabled passes
- [x] `procoder check` has no blocking findings for the resulting change.

## Evidence

- `go test ./internal/controller/ -run Console` before: FAIL, `rbac_manifest_test.go:343: no console Deployment` and `rbac_manifest_test.go:354: no ClusterRole kuvryn-sync-kuvryn-sync-console`; after: `--- PASS: TestHelmChartRendersASeparateConsole`, `--- PASS: TestConsoleClusterRoleOnlyImpersonates`. The existing chart tests (`TestHelmChartServicesSelectTheirOwnRelease`, `TestHelmChartUsesKuvrynSyncNames`) also pass.
- `TestSelfCheckReportsImpersonationOnHealthz` before: FAIL to compile, `s.SelfCheck undefined`; after: `--- PASS` (granted, users only, and none).
- `helm lint charts/kuvryn-sync --set console.enabled=true --set console.oidc.issuerURL=https://dex.example --set console.oidc.clientID=ksync`: "1 chart(s) linted, 0 chart(s) failed", and the same with the ingress enabled. `helm template` with a client secret, the ingress and connectors rendered the expected args, mounts and redirect URL.
- `make docs-check`: "72 relative links checked, 0 broken".
- `make test` exit 0; `make lint`: "0 issues."; `make manifests generate` left no diff; `npm --prefix web run build` ok; `make test-ui`: 6 passed, 3 skipped (the opt-in screenshots).
- `procoder check`: 0 blocking.
- Not done here: the README and docs screenshots, which wait for the real emblem files.
