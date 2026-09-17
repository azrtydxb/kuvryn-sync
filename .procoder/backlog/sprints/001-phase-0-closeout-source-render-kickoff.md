# Phase 0 closeout and source/render kickoff

Status: closed 2026-09-17
Created: 2026-09-17
Milestones: phase-0-foundation, phase-1-source-render
Spec: solder-full-product

## Goal

Close the foundation gap to a fully usable Solder development baseline, then start the first real product slice: resolving Git sources safely with Secret-backed auth and a reusable source cache. The sprint ends when Phase 0 stories are either done with evidence or explicitly carried, and the Phase 1 source subsystem has a tested foundation ready for renderers.

## Committed stories

### Phase 0 closeout

- [x] `20260917-001-controller-manager-builds-from-generated-scaffold.md` — Controller manager scaffold builds
- [x] `20260917-002-developer-workflow-runs-tests-and-lint.md` — Developer workflow tests/lint
- [x] `20260917-003-container-image-path-is-buildable.md` — Container image path — resolved by GHCR/KW image build evidence without local Docker
- [x] `20260917-004-repository-api-models-git-sources.md` — Repository API model
- [x] `20260917-005-application-api-separates-sync-and-health.md` — Application API sync/health split
- [x] `20260917-006-revision-api-records-auditable-attempts.md` — Revision API audit model
- [x] `20260917-007-samples-apply-against-generated-crds.md` — Samples and generated CRDs
- [x] `20260917-008-ci-runs-test-lint-and-image-checks.md` — CI test/lint/image checks
- [x] `20260917-009-local-kind-development-loop-works.md` — KW development loop — resolved by KW validation path per repository constraint against local Docker
- [x] `20260917-010-security-baseline-blocks-obvious-leaks.md` — Security baseline

### Phase 1 source kickoff

- [x] `20260917-011-repository-controller-resolves-git-revisions.md` — Git revision resolution
- [x] `20260917-012-git-auth-uses-kubernetes-secrets-safely.md` — Git auth via Secrets
- [x] `20260917-013-source-cache-reuses-repositories-safely.md` — Source cache reuse

## Execution plan

- [x] First verify existing Phase 0 work against each story rather than rewriting scaffold code that already exists.
- [x] Close the foundation defects that block repeatable local development: CI timeouts/concurrency, security/dependency gate, generated manifests, samples, image build, Kind loop, and documentation.
- [x] Implement source interfaces and Repository reconciliation as a narrow vertical slice before renderers: resolve refs, load auth from Secrets, fetch/cache repositories, update status and Events without leaking credentials.
- [x] Add unit/envtest coverage for missing credentials, bad refs, cache reuse, concurrent fetch coordination, and status/error classification.
- [x] Run `make test`, `procoder test`, `procoder lint`, `procoder security`, and `procoder check` before reporting the sprint as complete.

## Sprint acceptance

- [x] All committed Phase 0 stories have checked acceptance criteria and evidence, or are carried with a concrete reason.
- [x] Repository source resolution can resolve a configured Git ref to an observed immutable revision in tests.
- [x] Secret-backed Git auth paths are covered without logging or surfacing credential values.
- [x] Source cache behavior is tested for reuse and safe cache-loss semantics.
- [x] No blocking findings remain in `procoder check`; `procoder test` passes.

## Risks and watchpoints

- Kubebuilder-generated files must be regenerated through `make manifests generate`, not hand-edited.
- Do not introduce a repo-server or database while implementing source cache; local cache is performance-only.
- Do not log Secret payloads in test failures, controller logs, Events, or status.
- If real remote Git fixtures become flaky, keep deterministic local Git fixtures for tests and reserve network smoke tests for optional integration.

## Carry policy

Anything not meeting its acceptance criteria by sprint end is carried explicitly with reason and next evidence needed. No unchecked Phase 0 story is silently treated as done.

## Result

committed: 13
done: 13 (20260917-001-controller-manager-builds-from-generated-scaffold, 20260917-002-developer-workflow-runs-tests-and-lint, 20260917-003-container-image-path-is-buildable, 20260917-004-repository-api-models-git-sources, 20260917-005-application-api-separates-sync-and-health, 20260917-006-revision-api-records-auditable-attempts, 20260917-007-samples-apply-against-generated-crds, 20260917-008-ci-runs-test-lint-and-image-checks, 20260917-009-local-kind-development-loop-works, 20260917-010-security-baseline-blocks-obvious-leaks, 20260917-011-repository-controller-resolves-git-revisions, 20260917-012-git-auth-uses-kubernetes-secrets-safely, 20260917-013-source-cache-reuses-repositories-safely)
carried: 0 (the two originally carried Phase 0 stories were resolved by KW/GHCR evidence without local Docker)

## Retro

Docker-dependent verification slowed the sprint down because this machine does not have a running Docker daemon, so image-build and local Kind evidence had to be carried while CI coverage was improved.

Next sprint should keep Docker/Kind stories out of the critical path unless a container runtime is available, and should use deterministic local Git/render fixtures for source/render work.

Keep the pattern of closing implementation slices with direct command evidence plus story evidence updates before moving to the next sprint.
