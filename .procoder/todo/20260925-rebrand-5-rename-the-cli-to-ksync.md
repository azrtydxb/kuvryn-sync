# Rebrand 5: Rename the CLI to ksync

Status: closed 2026-09-25
Created: 2026-09-25
Plan: .procoder/plans/kuvryn-sync-rebrand.md

## Description

CLI usage and version, the notification approve command, the Makefile binary and the Dockerfile entrypoint.

## Acceptance criteria

- [x] `TestKsyncVersionAndHelp` fails first, then passes
- [x] `make test` passes
- [x] `procoder check` has no blocking findings for the resulting change.

## Evidence

- `go test ./internal/cli/ -run TestKsyncVersionAndHelp` before: FAIL, `version = "solder dev\n", want ksync <version>`; after: `ok`.
- `make test` exit 0 (first run caught `TestHelpListsEveryCommand` expecting the old unprefixed layout; updated to ` ksync <cmd>`); `make lint`: "0 issues."; `make build` wrote `bin/ksync`, and `bin/ksync version` printed `ksync sha-09c2a50-dirty`.
- `procoder check`: 0 blocking.
