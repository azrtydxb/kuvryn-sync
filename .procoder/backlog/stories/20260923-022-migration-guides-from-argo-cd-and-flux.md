# Migration guides from Argo CD and Flux

Status: done 2026-09-23
Created: 2026-09-23
Epic: ownership-adoption-and-migration
Sprint: -

## Description

As an operator, I follow a documented path to move an existing app from Argo CD or Flux to Solder without downtime.

## Acceptance criteria

- [x] docs include step-by-step guides for both, covering disabling the old controller's prune, adoption, and verification.
- [x] Each guide is exercised once in an e2e or scripted Kind run, recorded as evidence.

## Evidence

- Guides: docs/migrate-flux.md and docs/migrate-argocd.md cover suspending the old controller and disabling its prune (Flux `prune: false`, Argo CD finalizer removal) before deletion, adoption with `conflictPolicy: adopt` and approval, removing the old object, clearing leftover co-ownership by Solder's `solder.io/application` label, and settling back to `fail`.
- Exercised on Kind with the real tools following each guide command for command (`hack/migration/migrate-flux.sh`, `hack/migration/migrate-argocd.sh`): Flux kustomize-controller and Argo CD (stable manifests) deploy the fixture, the migration keeps the ConfigMap's UID (never recreated), the Solder Application is Synced/Healthy before and after settling, and no field ownership by the previous manager remains.
- The runs found and fixed: whole-object planning (fields owned by others or server defaults looked like permanent changes; fixed in the planner with managedFields ownership), `xargs -I{}` corrupting the `[{}]` patch in the guide command, and Argo CD 3.x annotation tracking defeating a label selector (the guides now select by Solder's own label).
- Gates: `make test`, `make lint` 0 issues, `procoder check` clean.
