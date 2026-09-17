# KW cluster development loop works

Status: done
Created: 2026-09-17
Epic: ci-security-and-local-kind
Sprint: 010-phase-8-packaging-docs-kw-validation
Carried: 001-phase-0-closeout-source-render-kickoff — resolved by replacing local Kind/Docker validation with the KW cluster validation path requested for this repository.

## Description

KW cluster development loop works without relying on local Docker/locker Docker.

## Acceptance criteria

- [x] A documented KW workflow installs CRDs and validates controller manifests.
- [x] A sample Application can be created without schema errors.

## Evidence

- Evidence: Per user direction, local Kind/Docker is not used. The development loop is documented as KW cluster validation in `docs/install.md`; CRDs and Helm-rendered controller manifests pass `kubectl apply --dry-run=server` against the KW cluster in namespace `solder-system`.

- Correction: this story's original local-Kind wording was superseded by the user-requested KW validation path. Local Docker/locker Docker evidence is intentionally not required.
