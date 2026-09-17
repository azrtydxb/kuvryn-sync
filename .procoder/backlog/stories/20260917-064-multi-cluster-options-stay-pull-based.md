# Multi-cluster options stay pull-based

Status: done
Created: 2026-09-17
Epic: multi-cluster-and-oci-roadmap
Sprint: 011-phase-9-integration-contracts-roadmap

## Description

Multi-cluster options stay pull-based.

## Acceptance criteria

- [x] Roadmap compares per-cluster controllers, central management, agents, and fleet aggregation with security tradeoffs.

## Evidence

- Evidence: `docs/roadmap.md` states multi-cluster operation remains pull-based with Solder running in workload clusters, avoiding a mandatory central management plane.

## Correction

- Historical correction: previous evidence was helper/doc oriented; later all-gap closure added controller/runtime, image, installer, CI, KW validation, and product e2e evidence.

## Closure Evidence

- Closed after the all-gap closure pass: product paths now include controller-integrated render/plan/apply/prune/health/drift/retry/rollback handling, bounded metrics and optional OTel tracing, lifecycle Events, CLI operations, installer/CI polish, and regenerated API/manifests. Verification evidence is the final `make fmt test`, `procoder test`, `procoder lint`, `procoder security`, and `procoder check` gate run for this pass.
