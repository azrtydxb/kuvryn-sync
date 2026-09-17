# Developer workflow runs tests and lint

Status: done
Created: 2026-09-17
Epic: foundation-control-plane
Sprint: 001-phase-0-closeout-source-render-kickoff

## Description

Developer workflow runs tests and lint.

## Acceptance criteria

- [x] `make test` runs envtest controller tests.
- [x] `make lint` and security reports are configured.
- [x] Go formatting is reproducible.

## Evidence

- Evidence: `make test` PASS; `procoder lint` 0 findings; Go formatting run through `make fmt`; Procoder check reports 0 blocking findings.
