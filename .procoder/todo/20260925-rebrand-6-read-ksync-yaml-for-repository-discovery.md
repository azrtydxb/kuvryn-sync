# Rebrand 6: Read .ksync.yaml for repository discovery

Status: open
Created: 2026-09-25
Plan: .procoder/plans/kuvryn-sync-rebrand.md

## Description

The discovery file name becomes .ksync.yaml, and the helpers are renamed.

## Acceptance criteria

- [ ] `TestDiscoveryReadsKsyncYaml` fails first, then passes
- [ ] `make test` passes
- [ ] `procoder check` has no blocking findings for the resulting change.

## Evidence
