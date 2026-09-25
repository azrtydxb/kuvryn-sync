# Rebrand 9: Rewrite the documentation and forbid the old name

Status: closed 2026-09-25
Created: 2026-09-25
Plan: .procoder/plans/kuvryn-sync-rebrand.md

## Description

Docs, README, samples, and the full spec renamed to kuvryn-sync-full-spec.md, plus the CHANGELOG clean-break entry and the upgrade note.

## Acceptance criteria

- [x] `TestNoSolderNameRemains` widened to the whole repository fails first, then passes
- [x] The link check reports 0 broken links and `make lint` passes
- [x] `procoder check` has no blocking findings for the resulting change.

## Evidence

- `go test ./internal/brand/` with the widened guard before the rewrite: FAIL, "the old product name remains outside its history (658 lines)", listing `.agents/rules/procoder.md:1:# solder - AI Agent Guide`, `solder-full-spec.md` (120), `docs/operations.md` (78), `docs/api.md` (55) and the other docs, agent files and Go comments; after: `ok`.
- `python3 hack/check-links.py docs README.md`: "61 relative links checked, 0 broken", exit 0. The root documents give "3 relative links checked, 0 broken". A scratch file with a missing file and an unknown `#anchor` reports both, and exits 1.
- `make test` exit 0; `make lint`: "0 issues."; `go vet -tags e2e ./test/e2e/` clean; `make manifests generate` after staging left no diff; `procoder agents --all` reports no drifted host file.
- `procoder check`: 0 blocking.
