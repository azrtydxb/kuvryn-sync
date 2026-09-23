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
  applicationServiceAccountName: payments-deployer
  pollInterval: 60s
```

### Spec fields

| Field                                | Description                                                                                                              |
| ------------------------------------ | ------------------------------------------------------------------------------------------------------------------------ |
| `spec.type`                          | Source adapter. `v1alpha1` supports `git`.                                                                               |
| `spec.git.url`                       | Git remote URL using `https`, `http`, `ssh` (including `git@host:path`), or `git`. Filesystem paths are rejected.        |
| `spec.git.revision`                  | Default branch, tag, or exact commit for Applications that omit a revision.                                              |
| `spec.git.auth.secretRef.name`       | Secret in the Repository namespace for private Git credentials; it must be labelled `solder.io/git-credentials: "true"`. |
| `spec.applicationConfigPaths`        | Repository-relative `.solder.yaml` paths. Defaults to root `.solder.yaml`.                                               |
| `spec.applicationServiceAccountName` | Service account discovered Applications run as. When empty, they use the controller's default service account.           |
| `spec.pollInterval`                  | Polling interval when no external wake-up signal exists.                                                                 |

### Status fields

| Field                     | Description                                      |
| ------------------------- | ------------------------------------------------ |
| `status.state`            | `Unknown`, `Ready`, or `Failed`.                 |
| `status.observedRevision` | Latest resolved source revision.                 |
| `status.lastFetchedAt`    | Time of last successful source fetch/inspection. |
| `status.conditions`       | Kubernetes Conditions for source readiness.      |

### Repository `.solder.yaml` discovery

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
- `spec.serviceAccountName` may only name the Repository's
  `spec.applicationServiceAccountName`, and defaults to it. When the Repository
  sets none, discovered Applications may not set a service account and use the
  controller's default. This keeps Git write access from choosing which
  service account Solder acts as.
- `applicationConfigPaths` entries must be repository-relative paths named
  `.solder.yaml`, must be unique, and must not escape the repository.
- Application names must be unique across all configured files.
- Discovered Applications are labeled with `solder.io/repository` and annotated
  with `solder.io/discovered-from` set to the source config path.
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
  serviceAccountName: payments-deployer
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

| Field                                     | Description                                                                                                                                                                                                                            |
| ----------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `spec.notifications[]`                    | Subscriptions: `sinkRef.name` of a NotificationSink and the `events` to send (`AwaitingApproval`, `Healthy`, `Failed`, `RolledBack`).                                                                                                  |
| `spec.serviceAccountName`                 | Service account Solder impersonates for this Application.                                                                                                                                                                              |
| `spec.destination.namespace`              | Namespace for namespaced desired resources: objects without one are placed there, objects naming another are rejected. Cluster-scoped kinds, including CRD kinds rendered alongside their CustomResourceDefinition, keep no namespace. |
| `spec.sync.automatic`                     | Apply approved plans automatically.                                                                                                                                                                                                    |
| `spec.sync.prune`                         | Delete previously managed resources removed from desired state.                                                                                                                                                                        |
| `spec.sync.selfHeal`                      | Correct managed live drift.                                                                                                                                                                                                            |
| `spec.sync.conflictPolicy`                | SSA conflict behavior. `v1alpha1` supports `fail`.                                                                                                                                                                                     |
| `spec.strategy.type`                      | Deployment strategy. `v1alpha1` supports rolling semantics.                                                                                                                                                                            |
| `spec.strategy.failurePolicy.action`      | Failure action such as rollback.                                                                                                                                                                                                       |
| `spec.strategy.failurePolicy.timeout`     | Bounds failure/health observation.                                                                                                                                                                                                     |
| `spec.strategy.failurePolicy.maxAttempts` | Retry-loop protection.                                                                                                                                                                                                                 |
| `spec.health.timeout`                     | Health observation timeout.                                                                                                                                                                                                            |
| `spec.history.limit`                      | Maximum retained Revisions.                                                                                                                                                                                                            |
| `spec.deletionPolicy`                     | `Orphan` or `DeleteManagedResources`.                                                                                                                                                                                                  |
| `spec.suspend`                            | Stop mutations while retaining status.                                                                                                                                                                                                 |

### Status fields

| Field                       | Description                                                                                            |
| --------------------------- | ------------------------------------------------------------------------------------------------------ |
| `status.state`              | High-level health state.                                                                               |
| `status.desiredRevision`    | Source revision Git asks Solder to run.                                                                |
| `status.deployedRevision`   | Source revision currently deployed after rollback handling.                                            |
| `status.serviceAccountName` | Service account Solder last impersonated for this Application.                                         |
| `status.sync.state`         | `Unknown`, `Synced`, `OutOfSync`, `Drifted`, `Planning`, `AwaitingApproval`, `Applying`, or `Pruning`. |
| `status.health.state`       | `Unknown`, `Progressing`, `Healthy`, `Degraded`, or `Suspended`.                                       |
| `status.managedKinds`       | Kinds Solder last applied; used to prune and watch managed objects of any kind.                        |
| `status.resources`          | Bounded counts of healthy/progressing/degraded/unknown resources.                                      |
| `status.conditions`         | Kubernetes Conditions for reconciliation.                                                              |

## HealthCheck

`HealthCheck` is a cluster-scoped set of CEL rules that decides the health of
one kind, for kinds whose status kstatus conventions cannot describe.

```yaml
apiVersion: solder.io/v1alpha1
kind: HealthCheck
metadata:
  name: argoproj-rollout
spec:
  group: argoproj.io
  kind: Rollout
  rules:
    - expression: object.status.phase == "Degraded"
      state: Degraded
      message: Rollout is degraded
    - expression: object.status.phase == "Healthy"
      state: Healthy
    - expression: "true"
      state: Progressing
      message: Rollout is progressing
```

| Field                     | Description                                                          |
| ------------------------- | -------------------------------------------------------------------- |
| `spec.group`              | API group of the kind; empty for the core group.                     |
| `spec.kind`               | Kind the rules apply to.                                             |
| `spec.rules[].expression` | CEL over the live object, `object`, returning a bool.                |
| `spec.rules[].state`      | `Healthy`, `Progressing`, or `Degraded` when the expression is true. |
| `spec.rules[].message`    | Message reported with the state.                                     |

Rules are evaluated in order, and across HealthChecks for the same kind in name
order; the first true expression decides. When none matches, kstatus
conventions apply. A validating webhook rejects expressions that do not
compile or do not return a bool. At runtime each rule has a cost limit; a rule
that errors or exceeds it reports the object as Progressing with reason
`HealthCheckFailed`, holding the rollout rather than passing it. Use `has()` to
guard fields that may be absent.

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
| `status.approval`                         | Audit record of a manual approval: `approvedBy`, `approvedAt`, `planDigest`.                                                          |
| `status.plan.digest`                      | Digest of the desired state and full redacted plan; approvals bind to it.                                                             |
| `status.failure`                          | Deterministic failure reason, message, resource, and retryability.                                                                    |
| `status.conditions`                       | Kubernetes Conditions for the attempt.                                                                                                |

## Invariants

- Sync state and health state are separate.
- Revision status stores bounded plan summaries and redacted per-resource changes.
- Server-Side Apply conflicts fail by default.
- Secret values must not appear in status, CLI output, Events, logs, metrics, or diagnostics.
- Rollback uses normal plan/apply/observe machinery and points at a previous healthy Revision.
