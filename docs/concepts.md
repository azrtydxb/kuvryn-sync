---
title: Concepts
nav_order: 3
---

# Concepts

## Repository

A `Repository` describes a desired-state source. In `v1alpha1`, the source type
is Git. Solder resolves a branch, tag, or commit to an observed revision and
records source readiness in status.

Private Git authentication is referenced through Kubernetes Secrets. The API
stores references to credentials, never the credential values.

After resolving a Git revision, the Repository controller looks for configured
`.solder.yaml` files. By default it reads the repository root `.solder.yaml`; for
monorepos, `spec.applicationConfigPaths` can point at one or more nested
`.solder.yaml` files. These files are the GitOps entry point for Application
definitions: Solder creates, updates, and removes Applications that are managed
by that Repository label.

Each configured path must be repository-relative, must stay inside the checkout,
and must be named `.solder.yaml`. Each file can contain one `Application` object
or an `applications:` list. Application names must be unique across all files, and
discovered Applications are annotated with `solder.io/discovered-from` so
operators can see which Git config file owns them.

## Application

An `Application` describes a deployable unit:

- source Repository, revision, path, and renderer;
- the service account Solder acts as when it reads, applies, and prunes;
- destination namespace constraints;
- sync policy for automatic apply, pruning, self-heal, and conflict handling;
- health observation timeout;
- bounded Revision history;
- optional rollback-on-failure behavior.

Applications are the primary object operators watch with `kubectl get app` or
`solder apps`. They can be applied directly to the Kubernetes API, or declared
in the source repository's `.solder.yaml` files for Repository-driven GitOps
bootstrapping.

## Revision

A `Revision` records one resolved deployment attempt. It carries source identity,
render settings, bounded plan summary, health summary, attempts, previous
healthy revision, and failure classification.

Revision status is intentionally bounded and redacted. It is suitable for CLI,
Events, dashboards, and integrations without becoming an unbounded copy of every
manifest.

## Sync versus health

Solder keeps convergence and operational health separate:

- **Sync** answers whether live resources match rendered desired state.
- **Health** answers whether those resources are operationally ready.

An Application can be out of sync but healthy, synced but degraded, or planning
while still serving traffic from the previous healthy Revision.

Deployments, StatefulSets, DaemonSets, Pods, and Jobs have dedicated health
rules. Every other kind follows the kstatus conventions most controllers use:

- `status.observedGeneration` behind `metadata.generation` is Progressing;
- a `Stalled=True` condition is Degraded;
- a `Reconciling=True` condition, or a `Ready` condition that is not `True`, is
  Progressing;
- anything else, including an object with no status, is Healthy.

A [HealthCheck](api.md#healthcheck) overrides these rules for one kind with CEL
expressions. A rollout waits only for Progressing resources, until
`spec.health.timeout`.

## Render, normalize, validate, plan

The reconciliation pipeline is:

1. resolve Git source;
2. materialize a checked-out worktree;
3. render YAML, Kustomize, or Helm;
4. validate desired objects for safety and uniqueness;
5. read matching live resources;
6. normalize server-managed fields;
7. compute a deterministic plan;
8. persist the bounded plan on Revision status.

## Apply and prune

Solder applies with Kubernetes Server-Side Apply. Conflict policy currently
supports `fail`, which blocks ownership conflicts instead of force-taking fields.

When pruning is enabled, Solder deletes previously managed resources that are no
longer present in desired state. It finds them by label across every kind in
the Application's `status.managedKinds` inventory, so objects of any kind are
pruned, including after a controller restart. Destructive changes are
represented in the plan before mutation.

## Drift and self-heal

Solder can detect live drift by comparing normalized live state to desired state.
When `selfHeal` is enabled, drift is corrected through the same plan/apply path.

Solder notices drift immediately for kinds it watches: ConfigMaps, Secrets,
Services, Deployments, StatefulSets, and DaemonSets, plus any managed kind the
controller has been granted `list` and `watch` on. Watches are metadata-only.
Applications that manage other kinds are re-checked every
`--drift-resync-interval` (Helm value `driftResyncInterval`, default 5m). See
[Operations](operations.md#drift-detection-for-other-kinds) to grant watches.

## Rollback

Rollback chooses a previous healthy Revision, records rollback intent on the
Application, and runs normal reconciliation against that prior source revision.
This preserves the same validation, planning, apply, health, and event behavior
as a forward sync.

## Events, metrics, and tracing

Solder emits Kubernetes Events for lifecycle transitions, registers Prometheus
collectors with bounded labels, and has an optional OpenTelemetry tracing seam.
Public integrations should prefer CRDs, Conditions, Events, and metrics over
controller internals.
