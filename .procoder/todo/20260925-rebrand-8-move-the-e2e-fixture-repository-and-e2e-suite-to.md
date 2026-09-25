# Rebrand 8: Move the e2e fixture repository and e2e suite to the new names

Status: closed 2026-09-25
Created: 2026-09-25
Plan: .procoder/plans/kuvryn-sync-rebrand.md

## Description

Rename azrtydxb/solder-e2e-app to azrtydxb/kuvryn-sync-e2e-app, push .ksync.yaml, and update test/e2e, the workflows and hack/migration.

## Acceptance criteria

- [x] `gh api repos/azrtydxb/kuvryn-sync-e2e-app` returns the renamed repository
- [x] CI E2E runs 8 of 8 specs with SUCCESS
- [x] `procoder check` has no blocking findings for the resulting change.

## Evidence

- `gh repo rename kuvryn-sync-e2e-app -R azrtydxb/solder-e2e-app --yes`, then `gh api repos/azrtydxb/kuvryn-sync-e2e-app --jq .full_name`: `azrtydxb/kuvryn-sync-e2e-app` (it returned 404 before the rename).
- The fixture had no `.solder.yaml` to rename; its commit 197a00c "Rename the fixture to Kuvryn Sync" (pushed to `main`) moves `solder.io/hook`, `solder.io/sync-wave` and `test.solder.io/case` to `sync.kuvryn.io`, and `solder-e2e*` names to `kuvryn-sync-e2e*`.
- CI on 72f2c2a (branch push, temporary trigger): E2E Tests run 36140390968 success, log "Ran 8 of 8 Specs in 169.740 seconds" and "SUCCESS! -- 8 Passed | 0 Failed"; Tests 36140390893, Lint 36140390844 and Image 36140390755 all success.
- Locally: `go vet -tags e2e ./test/e2e/` clean; `make test` exit 0; `make lint` "0 issues." (after wrapping two lines the longer names pushed past `lll`); `bin/kustomize build config/default` renders every name the suite asserts.
- `procoder check`: 0 blocking.
