# Rebrand 7: Rename the image, Helm chart, and kustomize install

Status: open
Created: 2026-09-25
Plan: .procoder/plans/kuvryn-sync-rebrand.md

## Description

charts/kuvryn-sync, namespace kuvryn-sync-system, namePrefix kuvryn-sync-, and image ghcr.io/azrtydxb/kuvryn-sync.

## Acceptance criteria

- [ ] `TestHelmChartUsesKuvrynSyncNames` fails first, then passes
- [ ] `helm lint charts/kuvryn-sync` and `make test` pass
- [ ] `procoder check` has no blocking findings for the resulting change.

## Evidence
