# Console 5: Scaffold the SPA on the vendored Azrty design system

Status: open
Created: 2026-09-25
Plan: .procoder/plans/kuvryn-sync-console.md

## Description

Vite, React and TS, the vendored DS components, the API client with 10s polling, theme, Makefile targets, Dockerfile node stage and CI build.

## Acceptance criteria

- [ ] The `getJSON` 401 Vitest fails first, then passes
- [ ] `npm --prefix web run build` creates internal/console/ui/dist/index.html
- [ ] `procoder check` has no blocking findings for the resulting change.

## Evidence
