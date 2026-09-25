# Console 8: Ship the console in the chart with docs and a self-check

Status: open
Created: 2026-09-25
Plan: .procoder/plans/kuvryn-sync-console.md

## Description

Chart values and templates, an impersonate-only ClusterRole, the SSAR self-check, docs/console.md with Dex, and the README.

## Acceptance criteria

- [ ] `TestHelmChartRendersASeparateConsole` and `TestConsoleClusterRoleOnlyImpersonates` fail first, then pass
- [ ] `helm lint` with the console enabled passes
- [ ] `procoder check` has no blocking findings for the resulting change.

## Evidence
