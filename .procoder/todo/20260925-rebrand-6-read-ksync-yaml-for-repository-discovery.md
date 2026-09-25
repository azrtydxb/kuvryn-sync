# Rebrand 6: Read .ksync.yaml for repository discovery

Status: closed 2026-09-25
Created: 2026-09-25
Plan: .procoder/plans/kuvryn-sync-rebrand.md

## Description

The discovery file name becomes .ksync.yaml, and the helpers are renamed.

## Acceptance criteria

- [x] `TestDiscoveryReadsKsyncYaml` fails first, then passes
- [x] `make test` passes
- [x] `procoder check` has no blocking findings for the resulting change.

## Evidence

- `go test ./internal/controller/ -run TestDiscoveryReadsKsyncYaml` before: build failed, "undefined: applicationsFromConfigFile" and "undefined: configPaths"; after the renames: `ok`.
- `make test` exit 0; `make manifests generate` after staging left no diff (CRD descriptions now say `.ksync.yaml`); `make lint`: "0 issues.".
- `procoder check`: 0 blocking.
