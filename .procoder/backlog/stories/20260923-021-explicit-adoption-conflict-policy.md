# Explicit adoption conflict policy

Status: open
Created: 2026-09-23
Epic: ownership-adoption-and-migration
Sprint: -

## Description

As an operator migrating to Solder, I opt an Application into taking ownership of fields another manager holds.

## Acceptance criteria

- [ ] `conflictPolicy` gains an explicit adopt/force value; the plan lists every field and previous manager being taken over.
- [ ] Adoption requires approval when manual approval is enabled; `fail` stays the default.
- [ ] Controller test covers adoption from a foreign field manager.

## Evidence

