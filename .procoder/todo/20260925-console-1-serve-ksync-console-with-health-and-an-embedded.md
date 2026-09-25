# Console 1: Serve ksync console with health and an embedded placeholder

Status: open
Created: 2026-09-25
Plan: .procoder/plans/kuvryn-sync-console.md

## Description

internal/console config and server, the ui embed with placeholder, and the ksync console subcommand.

## Acceptance criteria

- [ ] `TestServerServesHealthAndSecurityHeaders` fails first, then passes
- [ ] `ksync console --help` lists every flag
- [ ] `procoder check` has no blocking findings for the resulting change.

## Evidence
