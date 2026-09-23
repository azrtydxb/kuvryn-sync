# Pre- and post-sync hook Jobs

Status: open
Created: 2026-09-23
Epic: sync-hooks-and-waves
Sprint: -

## Description

As an application team, I run a migration Job before apply and a smoke-test Job after health.

## Acceptance criteria

- [ ] Resources annotated as pre-sync or post-sync hooks run at that stage; hook failure fails the Revision with the hook's name.
- [ ] Hook Jobs appear in the plan and in Revision status; completed hooks are cleaned up per a documented policy.

## Evidence

