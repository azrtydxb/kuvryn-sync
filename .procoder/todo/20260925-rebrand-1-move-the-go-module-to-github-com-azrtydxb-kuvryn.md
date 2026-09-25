# Rebrand 1: Move the Go module to github.com/azrtydxb/kuvryn-sync

Status: open
Created: 2026-09-25
Plan: .procoder/plans/kuvryn-sync-rebrand.md

## Description

go.mod, every import, and PROJECT repo/paths move to github.com/azrtydxb/kuvryn-sync.

## Acceptance criteria

- [ ] `TestNoSolderNameRemains` fails first on the old module path, then passes
- [ ] `go build ./...` and `make test` pass
- [ ] `procoder check` has no blocking findings for the resulting change.

## Evidence
