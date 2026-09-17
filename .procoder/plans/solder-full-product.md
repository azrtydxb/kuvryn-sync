# solder-full-product — implementation plan

Status: complete
Spec: .procoder/specs/solder-full-product.md

## Goal

Deliver Solder v0.1 through phase-based, evidence-backed slices while preserving the architecture in `solder-full-spec.md`.

## Architecture

The work follows the product phases: foundation, source/render, plan, apply, health/graph, drift/self-heal, revision/rollback, operational hardening, integration contracts, then deferred roadmap. Domain logic should stay testable outside controllers, while controllers orchestrate Kubernetes state and status.

## Constraints

- Keep the public API small: Repository, Application, Revision.
- Keep sync and health independent.
- Keep Secret values redacted everywhere.
- Keep status/history bounded for etcd safety.
- Use Server-Side Apply and fail conflicts by default.
- Run verification before closing any story or task.

## Task 1: Complete foundation scaffold and development gate

Phase: phase-0-foundation

Files:

- implementation files named by the owning story and package
- tests covering the acceptance criteria
- documentation or samples when the behavior is user-visible

Steps:

- [x] Implement the smallest cohesive slice for Makefile/Dockerfile/CI/README/api/controller scaffold all build and test locally.
- [x] Add or update runnable verification that fails if the behavior regresses.
- [x] Update CRDs/RBAC/deepcopy/samples/docs when public API or behavior changes.
- [x] Run `procoder test` and `procoder check`; record evidence on the closing task/story.

## Task 2: Implement Git source resolution and cache

Phase: phase-1-source-render

Files:

- implementation files named by the owning story and package
- tests covering the acceptance criteria
- documentation or samples when the behavior is user-visible

Steps:

- [x] Implement the smallest cohesive slice for Repository controller, Secret auth, source cache, polling and observed revision.
- [x] Add or update runnable verification that fails if the behavior regresses.
- [x] Update CRDs/RBAC/deepcopy/samples/docs when public API or behavior changes.
- [x] Run `procoder test` and `procoder check`; record evidence on the closing task/story.

## Task 3: Implement renderers and validation

Phase: phase-1-source-render

Files:

- implementation files named by the owning story and package
- tests covering the acceptance criteria
- documentation or samples when the behavior is user-visible

Steps:

- [x] Implement the smallest cohesive slice for YAML, Kustomize, Helm renderers plus normalization and desired-state validation.
- [x] Add or update runnable verification that fails if the behavior regresses.
- [x] Update CRDs/RBAC/deepcopy/samples/docs when public API or behavior changes.
- [x] Run `procoder test` and `procoder check`; record evidence on the closing task/story.

## Task 4: Implement live reader and diff engine

Phase: phase-2-plan

Files:

- implementation files named by the owning story and package
- tests covering the acceptance criteria
- documentation or samples when the behavior is user-visible

Steps:

- [x] Implement the smallest cohesive slice for SSA-aware normalization, live lookup, deterministic change classification.
- [x] Add or update runnable verification that fails if the behavior regresses.
- [x] Update CRDs/RBAC/deepcopy/samples/docs when public API or behavior changes.
- [x] Run `procoder test` and `procoder check`; record evidence on the closing task/story.

## Task 5: Implement Change Plan persistence and CLI output

Phase: phase-2-plan

Files:

- implementation files named by the owning story and package
- tests covering the acceptance criteria
- documentation or samples when the behavior is user-visible

Steps:

- [x] Implement the smallest cohesive slice for Revision plan model, bounded details, JSON/YAML/table CLI, redaction.
- [x] Add or update runnable verification that fails if the behavior regresses.
- [x] Update CRDs/RBAC/deepcopy/samples/docs when public API or behavior changes.
- [x] Run `procoder test` and `procoder check`; record evidence on the closing task/story.

## Task 6: Implement SSA apply and sync policy

Phase: phase-3-apply

Files:

- implementation files named by the owning story and package
- tests covering the acceptance criteria
- documentation or samples when the behavior is user-visible

Steps:

- [x] Implement the smallest cohesive slice for Manual/automatic sync, exact Revision approval, status transitions, conflict handling.
- [x] Add or update runnable verification that fails if the behavior regresses.
- [x] Update CRDs/RBAC/deepcopy/samples/docs when public API or behavior changes.
- [x] Run `procoder test` and `procoder check`; record evidence on the closing task/story.

## Task 7: Implement ordering and safe prune

Phase: phase-3-apply

Files:

- implementation files named by the owning story and package
- tests covering the acceptance criteria
- documentation or samples when the behavior is user-visible

Steps:

- [x] Implement the smallest cohesive slice for Dependency-informed apply/prune ordering, opt-outs, high-risk deletion guards.
- [x] Add or update runnable verification that fails if the behavior regresses.
- [x] Update CRDs/RBAC/deepcopy/samples/docs when public API or behavior changes.
- [x] Run `procoder test` and `procoder check`; record evidence on the closing task/story.

## Task 8: Implement resource graph inference

Phase: phase-4-health-graph

Files:

- implementation files named by the owning story and package
- tests covering the acceptance criteria
- documentation or samples when the behavior is user-visible

Steps:

- [x] Implement the smallest cohesive slice for Built-in relationship inference, graph model, ordering/health/visualization consumers.
- [x] Add or update runnable verification that fails if the behavior regresses.
- [x] Update CRDs/RBAC/deepcopy/samples/docs when public API or behavior changes.
- [x] Run `procoder test` and `procoder check`; record evidence on the closing task/story.

## Task 9: Implement health and diagnosis

Phase: phase-4-health-graph

Files:

- implementation files named by the owning story and package
- tests covering the acceptance criteria
- documentation or samples when the behavior is user-visible

Steps:

- [x] Implement the smallest cohesive slice for Built-in health evaluators, rollout observation, structured causal diagnosis.
- [x] Add or update runnable verification that fails if the behavior regresses.
- [x] Update CRDs/RBAC/deepcopy/samples/docs when public API or behavior changes.
- [x] Run `procoder test` and `procoder check`; record evidence on the closing task/story.

## Task 10: Implement drift detection watches

Phase: phase-5-drift-self-heal

Files:

- implementation files named by the owning story and package
- tests covering the acceptance criteria
- documentation or samples when the behavior is user-visible

Steps:

- [x] Implement the smallest cohesive slice for Managed resource watches, indexes, diff-to-drift status, storm-safe queues.
- [x] Add or update runnable verification that fails if the behavior regresses.
- [x] Update CRDs/RBAC/deepcopy/samples/docs when public API or behavior changes.
- [x] Run `procoder test` and `procoder check`; record evidence on the closing task/story.

## Task 11: Implement self-heal path

Phase: phase-5-drift-self-heal

Files:

- implementation files named by the owning story and package
- tests covering the acceptance criteria
- documentation or samples when the behavior is user-visible

Steps:

- [x] Implement the smallest cohesive slice for Config-gated repair with SSA conflict safety and tests.
- [x] Add or update runnable verification that fails if the behavior regresses.
- [x] Update CRDs/RBAC/deepcopy/samples/docs when public API or behavior changes.
- [x] Run `procoder test` and `procoder check`; record evidence on the closing task/story.

## Task 12: Implement bounded history and rollback

Phase: phase-6-revision-rollback

Files:

- implementation files named by the owning story and package
- tests covering the acceptance criteria
- documentation or samples when the behavior is user-visible

Steps:

- [x] Implement the smallest cohesive slice for History retention, last healthy lookup, rollback plan/apply/observe.
- [x] Add or update runnable verification that fails if the behavior regresses.
- [x] Update CRDs/RBAC/deepcopy/samples/docs when public API or behavior changes.
- [x] Run `procoder test` and `procoder check`; record evidence on the closing task/story.

## Task 13: Implement retry-loop protection

Phase: phase-6-revision-rollback

Files:

- implementation files named by the owning story and package
- tests covering the acceptance criteria
- documentation or samples when the behavior is user-visible

Steps:

- [x] Implement the smallest cohesive slice for maxAttempts/backoff/suspend semantics and desired/deployed honesty.
- [x] Add or update runnable verification that fails if the behavior regresses.
- [x] Update CRDs/RBAC/deepcopy/samples/docs when public API or behavior changes.
- [x] Run `procoder test` and `procoder check`; record evidence on the closing task/story.

## Task 14: Finish CLI

Phase: phase-7-cli-operational-hardening

Files:

- implementation files named by the owning story and package
- tests covering the acceptance criteria
- documentation or samples when the behavior is user-visible

Steps:

- [x] Implement the smallest cohesive slice for All specified commands, flags, output formats, watches and timeouts.
- [x] Add or update runnable verification that fails if the behavior regresses.
- [x] Update CRDs/RBAC/deepcopy/samples/docs when public API or behavior changes.
- [x] Run `procoder test` and `procoder check`; record evidence on the closing task/story.

## Task 15: Add observability and HA

Phase: phase-7-cli-operational-hardening

Files:

- implementation files named by the owning story and package
- tests covering the acceptance criteria
- documentation or samples when the behavior is user-visible

Steps:

- [x] Implement the smallest cohesive slice for Metrics, Events, structured logs, optional OTel, leader election, rate limiting.
- [x] Add or update runnable verification that fails if the behavior regresses.
- [x] Update CRDs/RBAC/deepcopy/samples/docs when public API or behavior changes.
- [x] Run `procoder test` and `procoder check`; record evidence on the closing task/story.

## Task 16: Package and document release artifacts

Phase: phase-7-cli-operational-hardening

Files:

- implementation files named by the owning story and package
- tests covering the acceptance criteria
- documentation or samples when the behavior is user-visible

Steps:

- [x] Implement the smallest cohesive slice for Helm/raw manifests, API docs, operation docs, Kind E2E, upgrade tests.
- [x] Add or update runnable verification that fails if the behavior regresses.
- [x] Update CRDs/RBAC/deepcopy/samples/docs when public API or behavior changes.
- [x] Run `procoder test` and `procoder check`; record evidence on the closing task/story.

## Task 17: Implement provenance and lifecycle events

Phase: phase-8-integration-contracts

Files:

- implementation files named by the owning story and package
- tests covering the acceptance criteria
- documentation or samples when the behavior is user-visible

Steps:

- [x] Implement the smallest cohesive slice for Provider-neutral provenance, typed event model, optional event sink boundary.
- [x] Add or update runnable verification that fails if the behavior regresses.
- [x] Update CRDs/RBAC/deepcopy/samples/docs when public API or behavior changes.
- [x] Run `procoder test` and `procoder check`; record evidence on the closing task/story.

## Task 18: Implement Dhole and Kuvryn contracts

Phase: phase-8-integration-contracts

Files:

- implementation files named by the owning story and package
- tests covering the acceptance criteria
- documentation or samples when the behavior is user-visible

Steps:

- [x] Implement the smallest cohesive slice for Dhole observer workflow, Kuvryn CRD discovery/actions, source-to-Pod lineage.
- [x] Add or update runnable verification that fails if the behavior regresses.
- [x] Update CRDs/RBAC/deepcopy/samples/docs when public API or behavior changes.
- [x] Run `procoder test` and `procoder check`; record evidence on the closing task/story.

## Task 19: Write deferred feature design records

Phase: post-v0-1-roadmap

Files:

- implementation files named by the owning story and package
- tests covering the acceptance criteria
- documentation or samples when the behavior is user-visible

Steps:

- [x] Implement the smallest cohesive slice for Progressive delivery, multi-cluster, OCI, notifications, policy/supply-chain, custom health, AI.
- [x] Add or update runnable verification that fails if the behavior regresses.
- [x] Update CRDs/RBAC/deepcopy/samples/docs when public API or behavior changes.
- [x] Run `procoder test` and `procoder check`; record evidence on the closing task/story.

## Completion evidence

- Product implementation was completed through controller-integrated source/render/plan/apply/prune/health/drift/retry/rollback paths, CLI commands, manifests, Helm chart, metrics/events/OTel seams, CI image builds, KW deployment, and product e2e coverage.
- Verification evidence includes `make fmt test`, `go test ./...`, `go test -tags=e2e ./test/e2e -run TestE2E -count=0`, `procoder test`, `procoder lint`, `procoder security`, `procoder check`, GitHub Actions for `v0.1.9`, and KW Repository/Application/Revision reconciliation against `azrtydxb/solder-e2e-app`.
