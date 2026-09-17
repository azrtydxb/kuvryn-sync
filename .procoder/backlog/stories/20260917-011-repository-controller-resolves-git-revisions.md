# Repository controller resolves Git revisions

Status: done
Created: 2026-09-17
Epic: git-source-and-cache
Sprint: 001-phase-0-closeout-source-render-kickoff

## Description

Repository controller resolves Git revisions.

## Acceptance criteria

- [x] HTTPS and SSH refs for branches, tags, and commits resolve to immutable revisions.
- [x] Status and Events report success/failure without secrets.

## Evidence

- Evidence: `internal/source/git/cache.go` resolves branches, tags, and exact commits through a serialized local Git cache. `internal/source/git/cache_test.go` proves branch/tag/commit resolution. `internal/controller/repository_controller.go` updates Repository status and Events with safe messages.
