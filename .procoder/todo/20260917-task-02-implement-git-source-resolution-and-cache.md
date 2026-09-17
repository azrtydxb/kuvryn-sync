# Task 2: Implement Git source resolution and cache

Status: done
Created: 2026-09-17
Plan: .procoder/plans/solder-full-product.md

## Description

Repository controller, Secret auth, source cache, polling and observed revision.

## Acceptance criteria

- [x] Work is mapped to one or more backlog stories.
- [x] Implementation files and tests are identified before coding the task.
- [x] Public API, samples, docs, and generated artifacts are updated when touched.
- [x] `procoder test` passes for the resulting change.
- [x] `procoder check` has no blocking findings for the resulting change.

## Evidence

- Repository product path resolves Git branches/tags/commits through `internal/source/git.Cache`, loads Kubernetes Secret auth without writing credentials to status/events/logs, records immutable `status.observedRevision`, emits safe Events, and reuses the cache under concurrent reconciles. Covered by repository controller and git cache tests; this is Phase 1 source evidence only, not evidence for renderer/planner/apply product integration.
