# Rebrand 1: Move the Go module to github.com/azrtydxb/kuvryn-sync

Status: closed 2026-09-25
Created: 2026-09-25
Plan: .procoder/plans/kuvryn-sync-rebrand.md

## Description

go.mod, every import, and PROJECT repo/paths move to github.com/azrtydxb/kuvryn-sync.

## Acceptance criteria

- [x] `TestNoSolderNameRemains` fails first on the old module path, then passes
- [x] `go build ./...` and `make test` pass
- [x] `procoder check` has no blocking findings for the resulting change.

## Evidence

- `go test ./internal/brand/` before the rename: FAIL, "old module path remains:" listing `PROJECT:10:repo: github.com/azrtydxb/solder` and every import; after: `ok github.com/azrtydxb/kuvryn-sync/internal/brand`.
- `go build ./...` succeeded; `go vet -tags e2e ./test/e2e/` clean; `make test` exit 0, every package ok; `make lint` reported "0 issues.".
- `procoder check` on the staged change: "0 blocking".
