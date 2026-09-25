# Rebrand 3: Move labels, annotations, and finalizers to sync.kuvryn.io/

Status: open
Created: 2026-09-25
Plan: .procoder/plans/kuvryn-sync-rebrand.md

## Description

Every solder.io/ key moves to sync.kuvryn.io/ with the same suffix.

## Acceptance criteria

- [ ] `TestNoSolderNameRemains` key-prefix check fails first, then passes
- [ ] `make test` passes
- [ ] `procoder check` has no blocking findings for the resulting change.

## Evidence
