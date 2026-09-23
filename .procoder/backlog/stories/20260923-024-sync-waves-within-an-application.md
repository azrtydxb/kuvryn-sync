# Sync waves within an Application

Status: done 2026-09-23
Created: 2026-09-23
Epic: sync-hooks-and-waves
Sprint: -

## Description

As an application team, I order groups of resources by wave annotation beyond the built-in kind order.

## Acceptance criteria

- [x] A wave annotation orders apply; each wave waits for Healthy before the next.
- [x] Kind ordering still applies within a wave; unit tests cover mixed waves and kinds.

## Evidence

- `solder.io/sync-wave` (and Argo CD's `sync-wave`) orders groups ascending; kind order applies within each wave (`TestGroupsOrderHooksWavesAndKinds` covers mixed waves, kinds, and hooks).
- `applies a later wave only once the earlier wave is Healthy`: a wave-1 ConfigMap is not created while the wave-0 Deployment is unavailable, and is applied once the Deployment reports available replicas.
- Found while testing: an Observing rollout was marked Healthy on the next reconcile once nothing was left to apply (fixed; `keeps observing an unready rollout instead of declaring it Healthy`, mutation checked), and managedFields list items were treated as owning whole lists, so defaulted container fields looked like changes (fixed; `TestListItemsAreMatchedByKeyNotOwnedWholesale`).
- Gates: `make test`, `make lint` 0 issues, `procoder check` clean.
