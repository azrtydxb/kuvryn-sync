# Phase 1 render and validation

Status: closed 2026-09-17
Created: 2026-09-17
Milestones: phase-1-source-render
Spec: solder-full-product

## Goal

Complete the render side of Phase 1 so Solder can turn fetched source workspaces into normalized Kubernetes objects and reject unsafe desired state before any planner or applier can mutate a cluster.

## Committed stories

- [x] `20260917-014-yaml-renderer-decodes-plain-manifests.md` — YAML renderer
- [x] `20260917-015-kustomize-renderer-normalizes-output.md` — Kustomize renderer
- [x] `20260917-016-helm-renderer-supports-values-files.md` — Helm renderer
- [x] `20260917-017-validation-rejects-unsafe-desired-state-early.md` — desired-state validation

## Execution plan

- [x] Define renderer interfaces and shared RenderInput/Result types.
- [x] Implement YAML, Kustomize, and Helm renderers with deterministic tests.
- [x] Add normalization and validation for malformed resources, missing metadata, duplicate identities, and namespace restrictions.
- [x] Keep renderer output as `[]unstructured.Unstructured` and avoid embedding planner/apply concerns.
- [x] Run `make test`, `procoder test`, `procoder lint`, `procoder security`, and `procoder check` before closing.

## Sprint acceptance

- [x] YAML, Kustomize, and Helm fixture workspaces render reproducibly.
- [x] Invalid desired state fails before mutation with clear validation reasons.
- [x] Duplicate identities and destination namespace violations are covered by tests.
- [x] No blocking findings remain in `procoder check`; `procoder test` passes.

## Retro

Renderer tests stayed deterministic by using fake Kustomize and Helm binaries instead of depending on local/CI tool installation.

Next sprint should keep planner tests similarly hermetic by using fake Kubernetes objects and controller-runtime clients before adding live-cluster tests.

Keep renderers narrow: they only produce unstructured Kubernetes objects, while validation and planning remain separate packages.

## Result

committed: 4
done: 4 (20260917-014-yaml-renderer-decodes-plain-manifests, 20260917-015-kustomize-renderer-normalizes-output, 20260917-016-helm-renderer-supports-values-files, 20260917-017-validation-rejects-unsafe-desired-state-early)
carried: 0

## Retro

<!-- What slowed us down this sprint. -->

<!-- What we change next sprint because of it. -->

<!-- One adaptation from this sprint worth keeping. -->
