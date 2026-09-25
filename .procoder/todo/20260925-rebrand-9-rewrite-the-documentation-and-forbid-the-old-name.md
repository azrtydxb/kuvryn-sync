# Rebrand 9: Rewrite the documentation and forbid the old name

Status: open
Created: 2026-09-25
Plan: .procoder/plans/kuvryn-sync-rebrand.md

## Description

Docs, README, samples, and the full spec renamed to kuvryn-sync-full-spec.md, plus the CHANGELOG clean-break entry and the upgrade note.

## Acceptance criteria

- [ ] `TestNoSolderNameRemains` widened to the whole repository fails first, then passes
- [ ] The link check reports 0 broken links and `make lint` passes
- [ ] `procoder check` has no blocking findings for the resulting change.

## Evidence
