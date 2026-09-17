# Source cache reuses repositories safely

Status: done
Created: 2026-09-17
Epic: git-source-and-cache
Sprint: 001-phase-0-closeout-source-render-kickoff

## Description

Source cache reuses repositories safely.

## Acceptance criteria

- [x] Concurrent fetches for one repository are serialized.
- [x] Applications sharing a Repository reuse local cache.
- [x] Cache eviction causes refetch only, not correctness loss.

## Evidence

- Evidence: `internal/source/git.Cache` keys cache directories by canonical URL and serializes fetches with per-cache locks. Tests cover concurrent Resolve calls sharing one cache dir and cache deletion causing a safe refetch with the same resolved revision.
