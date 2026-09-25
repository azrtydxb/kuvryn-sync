# Console 5: Scaffold the SPA on the vendored Azrty design system

Status: closed 2026-09-25
Created: 2026-09-25
Plan: .procoder/plans/kuvryn-sync-console.md

## Description

Vite, React and TS, the vendored DS components, the API client with 10s polling, theme, Makefile targets, Dockerfile node stage and CI build.

## Acceptance criteria

- [x] The `getJSON` 401 Vitest fails first, then passes
- [x] `npm --prefix web run build` creates internal/console/ui/dist/index.html
- [x] `procoder check` has no blocking findings for the resulting change.

## Evidence

- `npm --prefix web test` before: FAIL, `Error: Cannot find module './client' imported from .../web/src/api/client.test.ts`; after: `Tests  1 passed (1)`.
- `npm --prefix web run typecheck`: clean. `npm --prefix web run build`: wrote `internal/console/ui/dist/index.html`, the JS and CSS, and the fonts, lucide and emblems as files (no `data:` URIs), and restored `dist/.gitkeep`. `go test -count=1 ./internal/console/` with the dist present: `ok`.
- Emblems: untracked 640x640 placeholders listed in `.git/info/exclude`, not committed.
- `make test` exit 0; `make lint`: "0 issues."; `make manifests generate` left no diff.
- `procoder check`: 0 blocking (App.tsx formatted, doc comments added).
