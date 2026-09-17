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

## Application

An `Application` describes a deployable unit:

- source Repository, revision, path, and renderer;
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
longer present in desired state. Destructive changes are represented in the plan
before mutation.

## Drift and self-heal

Solder can detect live drift by comparing normalized live state to desired state.
When `selfHeal` is enabled, drift is corrected through the same plan/apply path.

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
