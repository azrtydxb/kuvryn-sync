# Rebrand 11: Release v0.4.0 and make the image public

Status: open
Created: 2026-09-25
Plan: .procoder/plans/kuvryn-sync-rebrand.md

## Description

Chart 0.4.0, the installer, CHANGELOG 0.4.0, the tag, the GitHub release and GHCR visibility public. This runs after the console plan.

## Acceptance criteria

- [ ] `TestReleaseImageIsPublic` fails before the visibility change, then passes
- [ ] The v0.4.0 image run on kw prints `ksync v0.4.0`
- [ ] `procoder check` has no blocking findings for the resulting change.

## Evidence
