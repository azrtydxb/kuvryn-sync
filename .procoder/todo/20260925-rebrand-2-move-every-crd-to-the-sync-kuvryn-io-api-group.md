# Rebrand 2: Move every CRD to the sync.kuvryn.io API group

Status: open
Created: 2026-09-25
Plan: .procoder/plans/kuvryn-sync-rebrand.md

## Description

groupName, rbac and webhook markers, PROJECT domain/group, regenerated CRDs, and chart rules.

## Acceptance criteria

- [ ] `TestCRDsUseTheKuvrynSyncGroup` fails first, then passes
- [ ] `make manifests generate` leaves no diff and `TestHelmChartRoleMatchesGeneratedRole` passes
- [ ] `procoder check` has no blocking findings for the resulting change.

## Evidence
