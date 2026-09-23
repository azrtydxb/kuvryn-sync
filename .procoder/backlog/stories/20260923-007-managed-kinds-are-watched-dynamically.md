# Managed kinds are watched dynamically

Status: done 2026-09-23
Created: 2026-09-23
Epic: generic-resource-support
Sprint: -

## Description

As an operator, I expect drift on any managed kind — Certificates, VirtualServices, custom CRs — to trigger a reconcile, not just the six hard-coded built-ins.

## Acceptance criteria

- [x] The Application controller starts metadata-only watches for each GVK it has applied, filtered by the Solder managed label, when a SelfSubjectAccessReview shows the controller may list/watch it; other kinds are re-checked every `--drift-resync-interval` (decided 2026-09-23).
- [x] Watches are started once per GVK and survive controller restarts via inventory rebuild.
- [x] An envtest test edits a managed custom resource and observes a reconcile enqueue for its Application.

## Evidence

- Watches: `internal/controller/application_watches.go` starts `source.Kind` watches on `PartialObjectMetadata` with a managed-label predicate after list+watch SSARs pass; denied kinds are rechecked once per resync interval. `records managed kinds and repairs a watched kind through its watch` runs a manager whose role is the generated one plus list/watch on widgets and recreates a deleted Widget within 5s; disabling the dynamic watch makes it fail (mutation checked).
- Resync: `repairs a kind the controller may not watch on the resync interval` recreates a deleted Gizmo (no grant) via the 8s resync; removing the resync requeue makes it fail (mutation checked).
- Inventory: new `status.managedKinds`, recorded after apply and when already synced; prune lists inventory ∪ desired kinds (legacy six only for Applications with no inventory yet). `prunes a custom-kind object removed from desired state using the inventory` fails when prune ignores the inventory (mutation checked).
- Once per GVK / restart: the watched set keeps one watch per GroupKind; on restart the first reconcile of each Application re-ensures watches for rendered ∪ inventory kinds. Covered by the code path, not by a dedicated restart test.
- Docs: concepts (drift, prune), operations (granting watches), api.md, CHANGELOG, Helm value `driftResyncInterval`. Gates: `make test`, `make lint` 0 issues, `procoder check` clean.
