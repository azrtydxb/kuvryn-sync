# Token 7: Document token sign-in

Status: closed 2026-09-25
Created: 2026-09-25
Plan: .procoder/plans/console-token-signin.md

## Description

Lead `docs/console.md` with token sign-in, add a viewer-token recipe with a
correct read-only role, a raw-manifest install section, and update
`docs/security.md` and the changelog (spec S-7).

## Acceptance criteria

- [x] `TestConsoleDocsCoverTokenSignIn` failed before the change and passes after it
- [x] The viewer-token section names a role that really grants read access to `sync.kuvryn.io` objects
- [x] `make docs-check` reports 0 broken links

## Evidence

- Red before 09807d0: `TestConsoleDocsCoverTokenSignIn` reported that docs/console.md had none of the three sections and that docs/security.md mentioned no token session.
- Green: `TestConsoleDocsCoverTokenSignIn` PASS.
- The viewer recipe binds the `kuvryn-sync-viewer` ClusterRole, which grants get and list on `sync.kuvryn.io` `*`. The docs say that the built-in `view` role does not cover sync.kuvryn.io and that the per-kind `kuvryn-sync-*-viewer-role` roles in dist/install.yaml carry no aggregation labels. The chart does not ship them.
- `make docs-check`: "82 relative links checked, 0 broken".
