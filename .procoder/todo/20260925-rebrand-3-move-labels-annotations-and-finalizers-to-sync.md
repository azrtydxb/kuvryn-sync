# Rebrand 3: Move labels, annotations, and finalizers to sync.kuvryn.io/

Status: closed 2026-09-25
Created: 2026-09-25
Plan: .procoder/plans/kuvryn-sync-rebrand.md

## Description

Every solder.io/ key moves to sync.kuvryn.io/ with the same suffix.

## Acceptance criteria

- [x] `TestNoSolderNameRemains` key-prefix check fails first, then passes
- [x] `make test` passes
- [x] `procoder check` has no blocking findings for the resulting change.

## Evidence

- `go test ./internal/brand/` with the new `solder\.io/` grep: FAIL, "old key prefix remains:" starting `api/v1alpha1/application_types.go:30: ApprovedRevisionAnnotation = "solder.io/approved-revision"` (83 lines); after the sed rewrite: `ok`.
- Concatenation check `git grep -E '"solder[^"]*"\s*\+|\+\s*"solder' -- '*.go'`: no output.
- `make test` exit 0; `make manifests generate` after staging left no diff; `make lint`: "0 issues."; `go vet -tags e2e ./test/e2e/` clean.
- `procoder check`: 0 blocking.
