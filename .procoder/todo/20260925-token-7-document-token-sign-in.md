# Token 7: Document token sign-in

Status: open
Created: 2026-09-25
Plan: .procoder/plans/console-token-signin.md

## Description

Lead `docs/console.md` with token sign-in, add a viewer-token recipe with a
correct read-only role, a raw-manifest install section, and update
`docs/security.md` and the changelog (spec S-7).

## Acceptance criteria

- [ ] `TestConsoleDocsCoverTokenSignIn` failed before the change and passes after it
- [ ] The viewer-token section names a role that really grants read access to `sync.kuvryn.io` objects
- [ ] `make docs-check` reports 0 broken links

## Evidence
