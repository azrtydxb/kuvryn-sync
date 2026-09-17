# Repository API models Git sources

Status: done
Created: 2026-09-17
Epic: api-schemas-and-samples
Sprint: 001-phase-0-closeout-source-render-kickoff

## Description

Repository API models Git sources.

## Acceptance criteria

- [x] Repository supports Git URL, default revision, auth Secret reference, polling, readiness, observed revision, and conditions.
- [x] Credentials cannot appear in status fields.

## Evidence

- Evidence: `api/v1alpha1/repository_types.go` models Git URL, revision, auth Secret ref, poll interval, state, observedRevision, lastFetchedAt and conditions. Credentials are loaded from Secrets in `internal/controller/repository_controller.go` and never written to status.
