# Phase 2 diff and plan core

Status: closed 2026-09-17
Created: 2026-09-17
Milestones: phase-2-plan
Spec: solder-full-product

## Goal

Build the core planner foundation: read live objects for rendered desired identities, normalize away expected server mutations, classify desired/live changes deterministically, and summarize bounded plan counts for Revisions.

## Committed stories

- [x] `20260917-018-live-reader-loads-desired-identities.md` — live reader
- [x] `20260917-019-normalizer-ignores-expected-server-mutations.md` — normalizer
- [x] `20260917-020-diff-classifies-create-update-delete-unchanged.md` — diff classification
- [x] `20260917-021-revision-stores-bounded-plan-summary.md` — bounded plan summary

## Supporting todo tasks

- [x] `.procoder/todo/20260917-task-04-implement-live-reader-and-diff-engine.md`
- [x] `.procoder/todo/20260917-task-05-implement-change-plan-persistence-and-cli-output.md`

## Execution plan

- [x] Implement resource identity helpers shared by validation, live reading, and planning.
- [x] Implement a Kubernetes live reader using controller-runtime clients for desired identities.
- [x] Implement normalization for status, resourceVersion, uid, generation, managedFields, creationTimestamp, and common annotations.
- [x] Implement deterministic create/update/delete/unchanged classification with bounded summaries.
- [x] Add unit tests around empty, create-only, update-only, delete-only, unchanged, and mixed plans.
- [x] Run gates before closing.

## Sprint acceptance

- [x] Live reader returns found and missing objects for desired identities.
- [x] Normalizer prevents false drift from expected server-populated fields.
- [x] Diff output is deterministic and correctly counts create/update/delete/unchanged.
- [x] Plan summary can be written into Revision status shape without unbounded data.
- [x] `procoder test` and `procoder check` pass with no blockers.

## Retro

The core planner stayed small because field-level diffs, redaction, and CLI presentation were kept out of the resource-level classification slice.

Next sprint should build on this by adding redacted field changes and operator-facing plan output without changing the resource action model.

Keep using fake clients and unstructured objects for fast planner feedback before adding cluster E2E coverage.

## Result

committed: 4
done: 4 (20260917-018-live-reader-loads-desired-identities, 20260917-019-normalizer-ignores-expected-server-mutations, 20260917-020-diff-classifies-create-update-delete-unchanged, 20260917-021-revision-stores-bounded-plan-summary)
carried: 0

## Retro

<!-- What slowed us down this sprint. -->

<!-- What we change next sprint because of it. -->

<!-- One adaptation from this sprint worth keeping. -->
