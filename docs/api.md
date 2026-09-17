---
title: API reference
nav_order: 5
---

# API reference

Solder exposes Kubernetes CRDs in API group `solder.io/v1alpha1`.

## Repository

`Repository` describes a desired-state source.

```yaml
apiVersion: solder.io/v1alpha1
kind: Repository
metadata:
  name: platform
  namespace: default
spec:
  type: git
  git:
    url: https://github.com/example/platform.git
    revision: main
  applicationConfigPaths:
    - .solder.yaml
  pollInterval: 60s
```

### Spec fields

| Field                          | Description                                                                 |
| ------------------------------ | --------------------------------------------------------------------------- |
| `spec.type`                    | Source adapter. `v1alpha1` supports `git`.                                  |
| `spec.git.url`                 | Git remote URL. HTTPS and SSH are supported by the source adapter.          |
| `spec.git.revision`            | Default branch, tag, or exact commit for Applications that omit a revision. |
| `spec.git.auth.secretRef.name` | Secret in the Repository namespace for private Git credentials.             |
| `spec.applicationConfigPaths`  | Repository-relative `.solder.yaml` paths. Defaults to root `.solder.yaml`.  |
| `spec.pollInterval`            | Polling interval when no external wake-up signal exists.                    |

### Status fields

| Field                     | Description                                      |
| ------------------------- | ------------------------------------------------ |
| `status.state`            | `Unknown`, `Ready`, or `Failed`.                 |
| `status.observedRevision` | Latest resolved source revision.                 |
| `status.lastFetchedAt`    | Time of last successful source fetch/inspection. |
| `status.conditions`       | Kubernetes Conditions for source readiness.      |

### Root `.solder.yaml`

When a Git Repository resolves, Solder checks configured `.solder.yaml` files.
When `spec.applicationConfigPaths` is empty, Solder reads the repository root
`.solder.yaml`. For monorepos or moved config, set one or more paths:

```yaml
spec:
  applicationConfigPaths:
    - teams/payments/.solder.yaml
    - teams/search/.solder.yaml
```

Each file can contain an Application list:

```yaml
applications:
  - metadata:
      name: payments
    spec:
      source:
        path: apps/payments
        render:
          type: kustomize
      destination:
        namespace: payments
      sync:
        automatic: true
        conflictPolicy: fail
```

For discovered Applications:

- `metadata.name` is required.
- `metadata.namespace`, when set, must match the Repository namespace.
- `spec.source.repositoryRef.name` defaults to the discovering Repository.
- `spec.source.render.type` is required.
- `applicationConfigPaths` entries must be repository-relative paths named
  `.solder.yaml` and must not escape the repository.
- Application names must be unique across all configured files.
- Applications managed by the same Repository label but removed from the
  configured `.solder.yaml` files are deleted.

A single full `Application` object is also accepted for small repositories.

## Application

`Application` is the main deployment abstraction.

```yaml
apiVersion: solder.io/v1alpha1
kind: Application
metadata:
  name: payments
  namespace: default
spec:
  source:
    repositoryRef:
      name: platform
    revision: main
    path: apps/payments
    render:
      type: kustomize
  destination:
    namespace: payments
  sync:
    automatic: true
    prune: true
    selfHeal: true
    conflictPolicy: fail
  strategy:
    type: rolling
    failurePolicy:
      action: rollback
      timeout: 5m
      maxAttempts: 2
  health:
    timeout: 5m
  history:
    limit: 20
  deletionPolicy: Orphan
```

### Source and render fields

| Field                                 | Description                                                  |
| ------------------------------------- | ------------------------------------------------------------ |
| `spec.source.repositoryRef.name`      | Repository in the same namespace.                            |
| `spec.source.revision`                | Branch, tag, or commit. Defaults to the Repository revision. |
| `spec.source.path`                    | Repository-relative desired-state path.                      |
| `spec.source.render.type`             | `yaml`, `kustomize`, or `helm`.                              |
| `spec.source.render.helm.releaseName` | Helm release name for template rendering.                    |
| `spec.source.render.helm.valuesFiles` | Repository-relative Helm values files.                       |

### Policy fields

| Field                                     | Description                                                     |
| ----------------------------------------- | --------------------------------------------------------------- |
| `spec.destination.namespace`              | Default namespace for namespaced desired resources.             |
| `spec.sync.automatic`                     | Apply approved plans automatically.                             |
| `spec.sync.prune`                         | Delete previously managed resources removed from desired state. |
| `spec.sync.selfHeal`                      | Correct managed live drift.                                     |
| `spec.sync.conflictPolicy`                | SSA conflict behavior. `v1alpha1` supports `fail`.              |
| `spec.strategy.type`                      | Deployment strategy. `v1alpha1` supports rolling semantics.     |
| `spec.strategy.failurePolicy.action`      | Failure action such as rollback.                                |
| `spec.strategy.failurePolicy.timeout`     | Bounds failure/health observation.                              |
| `spec.strategy.failurePolicy.maxAttempts` | Retry-loop protection.                                          |
| `spec.health.timeout`                     | Health observation timeout.                                     |
| `spec.history.limit`                      | Maximum retained Revisions.                                     |
| `spec.deletionPolicy`                     | `Orphan` or `DeleteManagedResources`.                           |
| `spec.suspend`                            | Stop mutations while retaining status.                          |

### Status fields

| Field                     | Description                                                                                            |
| ------------------------- | ------------------------------------------------------------------------------------------------------ |
| `status.state`            | High-level health state.                                                                               |
| `status.desiredRevision`  | Source revision Git asks Solder to run.                                                                |
| `status.deployedRevision` | Source revision currently deployed after rollback handling.                                            |
| `status.sync.state`       | `Unknown`, `Synced`, `OutOfSync`, `Drifted`, `Planning`, `AwaitingApproval`, `Applying`, or `Pruning`. |
| `status.health.state`     | `Unknown`, `Progressing`, `Healthy`, `Degraded`, or `Suspended`.                                       |
| `status.resources`        | Bounded counts of healthy/progressing/degraded/unknown resources.                                      |
| `status.conditions`       | Kubernetes Conditions for reconciliation.                                                              |

## Revision

`Revision` records an auditable deployment attempt.

### Spec fields

| Field                            | Description                                                        |
| -------------------------------- | ------------------------------------------------------------------ |
| `spec.applicationRef.name`       | Application that owns this deployment attempt.                     |
| `spec.source.repositoryRef.name` | Repository used for this attempt.                                  |
| `spec.source.revision`           | Resolved source revision, usually a full Git commit SHA.           |
| `spec.source.path`               | Rendered repository path.                                          |
| `spec.source.render`             | Renderer configuration used for this attempt.                      |
| `spec.provenance`                | Optional provider-neutral source, artifact, and pipeline evidence. |
| `spec.desiredStateHash`          | Deterministic rendered-state fingerprint.                          |

### Status fields

| Field                                     | Description                                                                                                                           |
| ----------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------- |
| `status.phase`                            | `Pending`, `Planning`, `AwaitingApproval`, `Applying`, `Observing`, `Healthy`, `Failed`, `RollingBack`, `RolledBack`, or `Cancelled`. |
| `status.startedAt` / `status.completedAt` | Attempt timing.                                                                                                                       |
| `status.attempts`                         | Retry-loop protection counter.                                                                                                        |
| `status.plan`                             | Bounded, redacted plan summary.                                                                                                       |
| `status.health`                           | Bounded resource health summary.                                                                                                      |
| `status.previousRevision`                 | Prior healthy Revision when known.                                                                                                    |
| `status.failure`                          | Deterministic failure reason, message, resource, and retryability.                                                                    |
| `status.conditions`                       | Kubernetes Conditions for the attempt.                                                                                                |

## Invariants

- Sync state and health state are separate.
- Revision status stores bounded plan summaries and redacted per-resource changes.
- Server-Side Apply conflicts fail by default.
- Secret values must not appear in status, CLI output, Events, logs, metrics, or diagnostics.
- Rollback uses normal plan/apply/observe machinery and points at a previous healthy Revision.
