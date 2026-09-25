# Rebrand 8: Move the e2e fixture repository and e2e suite to the new names

Status: open
Created: 2026-09-25
Plan: .procoder/plans/kuvryn-sync-rebrand.md

## Description

Rename azrtydxb/solder-e2e-app to azrtydxb/kuvryn-sync-e2e-app, push .ksync.yaml, and update test/e2e, the workflows and hack/migration.

## Acceptance criteria

- [ ] `gh api repos/azrtydxb/kuvryn-sync-e2e-app` returns the renamed repository
- [ ] CI E2E runs 8 of 8 specs with SUCCESS
- [ ] `procoder check` has no blocking findings for the resulting change.

## Evidence
