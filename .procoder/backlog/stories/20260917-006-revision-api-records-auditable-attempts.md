# Revision API records auditable attempts

Status: done
Created: 2026-09-17
Epic: api-schemas-and-samples
Sprint: 001-phase-0-closeout-source-render-kickoff

## Description

Revision API records auditable attempts.

## Acceptance criteria

- [x] Revision spec records application, source, provenance, and desired-state hash.
- [x] Revision status records phase, bounded plan, health, previous revision, and failure.

## Evidence

- Evidence: `api/v1alpha1/revision_types.go` records applicationRef, source, provenance, desiredStateHash, phase, bounded plan, health, previousRevision, and failure. Generated CRDs include Revision status schema.
