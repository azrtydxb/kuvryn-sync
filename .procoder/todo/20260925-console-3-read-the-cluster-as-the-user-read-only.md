# Console 3: Read the cluster as the user, read-only

Status: open
Created: 2026-09-25
Plan: .procoder/plans/kuvryn-sync-console.md

## Description

UserClient with impersonation and readOnlyTransport, which refuses writes, Secrets and system: identities.

## Acceptance criteria

- [ ] `TestConsoleClientIsReadOnly` fails first, then passes
- [ ] `procoder check` has no blocking findings for the resulting change.

## Evidence
