---
title: API reference
nav_order: 5
---

# API reference

Kuvryn Sync exposes Kubernetes CRDs in API group `sync.kuvryn.io/v1alpha1`.

## Repository

`Repository` describes a desired-state source.

```yaml
apiVersion: sync.kuvryn.io/v1alpha1
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
    - .ksync.yaml
  applicationServiceAccountName: payments-deployer
  pollInterval: 60s
```

### Spec fields

| Field                                | Description                                                                                                                   |
| ------------------------------------ | ----------------------------------------------------------------------------------------------------------------------------- |
| `spec.type`                          | Source adapter. `v1alpha1` supports `git`.                                                                                    |
| `spec.git.url`                       | Git remote URL using `https`, `http`, `ssh` (including `git@host:path`), or `git`. Filesystem paths are rejected.             |
| `spec.git.revision`                  | Default branch, tag, or exact commit for Applications that omit a revision.                                                   |
| `spec.git.auth.secretRef.name`       | Secret in the Repository namespace for private Git credentials; it must be labelled `sync.kuvryn.io/git-credentials: "true"`. |
| `spec.applicationConfigPaths`        | Repository-relative `.ksync.yaml` paths. Defaults to root `.ksync.yaml`.                                                      |
| `spec.applicationServiceAccountName` | Service account discovered Applications run as. When empty, they use the controller's default service account.                |
| `spec.pollInterval`                  | Polling interval when no external wake-up signal exists.                                                                      |
| `spec.webhook.secretRef.name`        | Secret whose `token` authenticates GitHub/GitLab push webhooks for this Repository.                                           |
| `spec.imageUpdate`                   | Commit ImagePolicy selections back to Git: `secretRef` (push credentials), `branch`, `path`, `authorName`, `authorEmail`.     |

### Status fields

| Field                     | Description                                      |
| ------------------------- | ------------------------------------------------ |
| `status.state`            | `Unknown`, `Ready`, or `Failed`.                 |
| `status.observedRevision` | Latest resolved source revision.                 |
| `status.lastFetchedAt`    | Time of last successful source fetch/inspection. |
| `status.conditions`       | Kubernetes Conditions for source readiness.      |

### Repository `.ksync.yaml` discovery

When a Git Repository resolves, Kuvryn Sync checks configured `.ksync.yaml` files.
When `spec.applicationConfigPaths` is empty, Kuvryn Sync reads the repository root
`.ksync.yaml`. For monorepos or moved config, set one or more paths:

```yaml
spec:
  applicationConfigPaths:
    - teams/payments/.ksync.yaml
    - teams/search/.ksync.yaml
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
  service account Kuvryn Sync acts as.
- `applicationConfigPaths` entries must be repository-relative paths named
  `.ksync.yaml`, must be unique, and must not escape the repository.
- Application names must be unique across all configured files.
- Discovered Applications are labeled with `sync.kuvryn.io/repository` and annotated
  with `sync.kuvryn.io/discovered-from` set to the source config path.
- Applications managed by the same Repository label but removed from the
  configured `.ksync.yaml` files are deleted.

A single full `Application` object is also accepted for small repositories.

## Application

`Application` is the main deployment abstraction.

```yaml
apiVersion: sync.kuvryn.io/v1alpha1
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
  health:
    timeout: 5m
  history:
    limit: 20
  deletionPolicy: Orphan
```

### Source and render fields

| Field                                  | Description                                                                                                                                                                                                                                                                                                                                                                                                         |
| -------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `spec.source.repositoryRef.name`       | Repository in the same namespace.                                                                                                                                                                                                                                                                                                                                                                                   |
| `spec.source.revision`                 | Branch, tag, or commit. Defaults to the Repository revision.                                                                                                                                                                                                                                                                                                                                                        |
| `spec.source.path`                     | Repository-relative desired-state path.                                                                                                                                                                                                                                                                                                                                                                             |
| `spec.source.render.type`              | `yaml`, `kustomize`, or `helm`.                                                                                                                                                                                                                                                                                                                                                                                     |
| `spec.source.render.helm.releaseName`  | Helm release name for template rendering. Defaults to `kuvryn-sync`. Like Helm, it must be a lowercase DNS subdomain (lowercase letters, digits, `-` and `.`, each dot-separated part starting and ending with a letter or digit) of at most 53 characters. The API server rejects any other name, and an Application stored with one before this rule fails with `ValidationFailure` until the release is renamed. |
| `spec.source.render.helm.valuesFiles`  | Repository-relative Helm values files.                                                                                                                                                                                                                                                                                                                                                                              |
| `spec.source.render.helm.chart`        | Pull `name` at exact `version` from an `https://` Helm repository or `oci://` registry (`repository`), with optional `secretRef` (`username`/`password`, labelled `sync.kuvryn.io/registry-credentials: "true"`). The archive digest is recorded in the Revision's `status.chartDigest`.                                                                                                                            |
| `spec.source.render.helm.valuesFrom[]` | `kind` (`ConfigMap` or `Secret`), `name`, and `key` (default `values.yaml`) in the Application namespace, read as the Application's service account. Values from Secrets are masked in plans.                                                                                                                                                                                                                       |
| `spec.source.render.helm.values`       | Inline values, merged last.                                                                                                                                                                                                                                                                                                                                                                                         |

### Policy fields

| Field                                     | Description                                                                                                                                                                                                                                                              |
| ----------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `spec.decryption`                         | `provider: sops` and `secretRef.name` of a Secret labelled `sync.kuvryn.io/decryption-key: "true"` whose `.agekey` entries hold age private keys.                                                                                                                        |
| `spec.dependsOn[].name`                   | Applications in the same namespace that must be Healthy at their desired revision before this one applies. Cycles are reported as `DependenciesReady=False/DependencyCycle`.                                                                                             |
| `spec.notifications[]`                    | Subscriptions: `sinkRef.name` of a NotificationSink and the `events` to send (`AwaitingApproval`, `Healthy`, `Failed`, `RolledBack`).                                                                                                                                    |
| `spec.serviceAccountName`                 | Service account Kuvryn Sync impersonates for this Application.                                                                                                                                                                                                           |
| `spec.destination.namespace`              | Namespace for namespaced desired resources: objects without one are placed there, objects naming another are rejected. Cluster-scoped kinds, including CRD kinds rendered alongside their CustomResourceDefinition, keep no namespace.                                   |
| `spec.sync.automatic`                     | Apply approved plans automatically.                                                                                                                                                                                                                                      |
| `spec.sync.prune`                         | Delete previously managed resources removed from desired state, except those annotated `sync.kuvryn.io/prune: "disabled"` and high-risk kinds, which are skipped; see [Apply and prune](concepts.md#apply-and-prune).                                                    |
| `spec.sync.selfHeal`                      | Correct managed live drift.                                                                                                                                                                                                                                              |
| `spec.sync.conflictPolicy`                | `fail` (default) stops on SSA ownership conflicts; `adopt` takes over the conflicting fields, listing each field and previous manager in the plan.                                                                                                                       |
| `spec.strategy.type`                      | Deployment strategy. `v1alpha1` supports rolling semantics.                                                                                                                                                                                                              |
| `spec.strategy.failurePolicy.action`      | `pause` (default) holds the failed Revision; `rollback` returns to the previous healthy Revision and holds the failed one; see [Rollback](concepts.md#rollback).                                                                                                         |
| `spec.strategy.failurePolicy.timeout`     | Bounds failure/health observation.                                                                                                                                                                                                                                       |
| `spec.strategy.failurePolicy.maxAttempts` | Attempts a desired revision gets before retries stop with `RetryBlocked`; defaults to 1. Under `action: rollback` a failure rolls back at once and the failed revision is held, so it applies only when no earlier healthy Revision exists or the rollback is abandoned. |
| `spec.health.timeout`                     | Health observation timeout.                                                                                                                                                                                                                                              |
| `spec.history.limit`                      | Maximum retained Revisions. Revisions a rollback holds are never deleted and do not count.                                                                                                                                                                               |
| `spec.deletionPolicy`                     | `Orphan` or `DeleteManagedResources`.                                                                                                                                                                                                                                    |
| `spec.suspend`                            | Stop mutations while retaining status.                                                                                                                                                                                                                                   |

### Status fields

| Field                       | Description                                                                                            |
| --------------------------- | ------------------------------------------------------------------------------------------------------ |
| `status.state`              | High-level health state.                                                                               |
| `status.desiredRevision`    | Source revision Git asks Kuvryn Sync to run.                                                           |
| `status.deployedRevision`   | Source revision currently deployed after rollback handling.                                            |
| `status.serviceAccountName` | Service account Kuvryn Sync last impersonated for this Application.                                    |
| `status.sync.state`         | `Unknown`, `Synced`, `OutOfSync`, `Drifted`, `Planning`, `AwaitingApproval`, `Applying`, or `Pruning`. |
| `status.health.state`       | `Unknown`, `Progressing`, `Healthy`, `Degraded`, or `Suspended`.                                       |
| `status.managedKinds`       | Kinds Kuvryn Sync last applied; used to prune and watch managed objects of any kind.                   |
| `status.resources`          | Bounded counts of healthy/progressing/degraded/unknown resources.                                      |
| `status.diagnosis`          | Up to 10 root causes of unhealthy managed resources; empty when the Application is Healthy.            |
| `status.observedGeneration` | Latest `metadata.generation` processed.                                                                |
| `status.conditions`         | Kubernetes Conditions for reconciliation; see [Conditions](#conditions).                               |

### Diagnosis

`status.diagnosis` explains why an Application is not Healthy. Kuvryn Sync sets it
whenever it evaluates health and finds a managed resource that is not Healthy,
and clears it once every managed resource is. It is also cleared when
reconciliation fails before health is observed, such as with a
`SourceFailure`, `ServiceAccountFailure`, render, validation, apply or prune
failure, and when retries are blocked after such a failure. It is kept while
the Application reports drift, which does not observe health again, and while
retries are blocked after a health failure (`HealthFailure`, `HookFailed` or
`TimeoutFailure`), which it still explains. Each entry is one root cause:

| Field      | Meaning                                                                                                                  |
| ---------- | ------------------------------------------------------------------------------------------------------------------------ |
| `resource` | The resource at the root of the failure (`apiVersion`, `kind`, `namespace`, `name`), such as a missing Secret or a Pod.  |
| `reason`   | A CamelCase word naming the failure, such as `MissingSecret`, `ImagePullBackOff`, `CrashLoopBackOff` or `Unschedulable`. |
| `message`  | Redacted evidence, at most 512 characters.                                                                               |
| `chain`    | Up to 10 resources, from the unhealthy managed resource down to `resource`, both included.                               |

A root cause shared by several resources, such as one missing Secret that
three Pods need, is listed once. Degraded resources are explained first. See
[Reading a diagnosis](troubleshooting.md#reading-a-diagnosis).

```yaml
status:
  diagnosis:
    - reason: MissingSecret
      resource: { apiVersion: v1, kind: Secret, namespace: payments, name: db }
      message: >-
        Secret payments/db does not exist; Pod api-7d9f-x2k:
        CreateContainerConfigError: container api is waiting: secret "db" not found
      chain:
        - {
            apiVersion: apps/v1,
            kind: Deployment,
            namespace: payments,
            name: api,
          }
        - {
            apiVersion: apps/v1,
            kind: ReplicaSet,
            namespace: payments,
            name: api-7d9f,
          }
        - { apiVersion: v1, kind: Pod, namespace: payments, name: api-7d9f-x2k }
        - { apiVersion: v1, kind: Secret, namespace: payments, name: db }
```

## HealthCheck

`HealthCheck` is a cluster-scoped set of CEL rules that decides the health of
one kind, for kinds whose status kstatus conventions cannot describe.

```yaml
apiVersion: sync.kuvryn.io/v1alpha1
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

## NotificationSink

`NotificationSink` is a namespaced destination for Application lifecycle
notifications. Applications in the same namespace reference it from
`spec.notifications[].sinkRef`; see
[Notifications](operations.md#notifications).

```yaml
apiVersion: sync.kuvryn.io/v1alpha1
kind: NotificationSink
metadata:
  name: audit
spec:
  type: webhook
  secretRef:
    name: audit-webhook
```

| Field                 | Description                                                                                                           |
| --------------------- | --------------------------------------------------------------------------------------------------------------------- |
| `spec.type`           | `webhook` (a JSON body signed with HMAC-SHA256) or `slack` (a Slack incoming webhook).                                |
| `spec.secretRef.name` | Secret in the sink's namespace holding `url`, which must be `https`, and, for `webhook` sinks, `hmacKey` for signing. |

NotificationSink has no status. A missing sink or invalid Secret is reported on
the Application as `NotificationsReady=False`.

## ImagePolicy

`ImagePolicy` scans an image repository and selects the image to run; see
[Image automation](operations.md#image-automation).

```yaml
apiVersion: sync.kuvryn.io/v1alpha1
kind: ImagePolicy
metadata:
  name: api
spec:
  image: ghcr.io/acme/api
  interval: 5m
  policy:
    semver:
      range: ">=1.2.0 <2.0.0"
```

### Spec fields

| Field                          | Description                                                                                               |
| ------------------------------ | --------------------------------------------------------------------------------------------------------- |
| `spec.image`                   | Image repository to scan, such as `ghcr.io/acme/api`.                                                     |
| `spec.secretRef.name`          | Optional `kubernetes.io/dockerconfigjson` Secret, labelled `sync.kuvryn.io/registry-credentials: "true"`. |
| `spec.interval`                | How often the registry is scanned. Defaults to `5m`.                                                      |
| `spec.policy.semver.range`     | Select the highest tag within a semver constraint, such as `>=1.2.0 <2.0.0`.                              |
| `spec.policy.tagPattern.regex` | Select the last tag matching a regular expression.                                                        |
| `spec.policy.tagPattern.order` | `alphabetical` (default) or `numerical`, by the first capture group or the whole tag.                     |
| `spec.policy.digest.tag`       | Follow the current digest of one fixed tag, such as `main`.                                               |
| `spec.webhook.secretRef.name`  | Optional Secret whose `token` authenticates requests to `/hooks/imagepolicies/<namespace>/<name>`.        |

Set exactly one of `semver`, `tagPattern`, or `digest`.

### Status fields

| Field                       | Description                                           |
| --------------------------- | ----------------------------------------------------- |
| `status.latestTag`          | Selected tag.                                         |
| `status.latestDigest`       | Manifest digest of the selected tag.                  |
| `status.latestImage`        | Immutable reference, `image:tag@digest`.              |
| `status.lastScannedAt`      | When the registry was last read successfully.         |
| `status.observedGeneration` | Latest `metadata.generation` processed.               |
| `status.conditions`         | Kubernetes Conditions; `Ready` reports the selection. |

## Revision

`Revision` records an auditable deployment attempt.

### Spec fields

| Field                            | Description                                              |
| -------------------------------- | -------------------------------------------------------- |
| `spec.applicationRef.name`       | Application that owns this deployment attempt.           |
| `spec.source.repositoryRef.name` | Repository used for this attempt.                        |
| `spec.source.revision`           | Resolved source revision, usually a full Git commit SHA. |
| `spec.source.path`               | Rendered repository path.                                |
| `spec.source.render`             | Renderer configuration used for this attempt.            |
| `spec.desiredStateHash`          | Deterministic rendered-state fingerprint.                |

### Status fields

| Field                                     | Description                                                                                                                                           |
| ----------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------- |
| `status.phase`                            | `Pending`, `Planning`, `AwaitingApproval`, `Applying`, `Observing`, `Healthy`, `Failed`, `RollingBack`, `RolledBack`, or `Cancelled`.                 |
| `status.startedAt` / `status.completedAt` | Attempt timing.                                                                                                                                       |
| `status.attempts`                         | Retry-loop protection counter.                                                                                                                        |
| `status.plan`                             | Bounded, redacted plan summary.                                                                                                                       |
| `status.health`                           | Bounded resource health summary.                                                                                                                      |
| `status.previousRevision`                 | Prior healthy Revision when known.                                                                                                                    |
| `status.approval`                         | Audit record of a manual approval: `approvedBy`, `approvedAt`, `planDigest`, and `desiredStateHash`, which lets one approval cover the whole rollout. |
| `status.plan.digest`                      | Digest of the desired state and full redacted plan; approvals bind to it.                                                                             |
| `status.chartDigest`                      | sha256 digest of the Helm chart archive pulled for `render.helm.chart`.                                                                               |
| `status.hooks`                            | Up to 64 pre-sync and post-sync hooks run for this Revision, each with `resource`, `stage`, `state`, and `message`.                                   |
| `status.failure`                          | Deterministic failure reason, message, resource, and retryability.                                                                                    |
| `status.conditions`                       | Kubernetes Conditions for the attempt.                                                                                                                |

## Conditions

| Object      | Type                 | Meaning                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                |
| ----------- | -------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Repository  | `Ready`              | `True` with `FetchSucceeded` once the source resolves; `False` with the failure reason, such as `SourceFailure`, `AuthenticationFailure` or `ValidationFailure`.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       |
| Repository  | `ImagesUpdated`      | Image write-back result: `True` with `UpToDate` or `Committed`, `False` with `UpdateFailed`, or `Disabled` when the manager runs without image write-back.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| Application | `Ready`              | `True` with `Healthy` after the last rollout completed Synced and Healthy. `False` with a [failure reason](#failure-reasons) when the Application cannot start a rollout (an invalid spec, its service account, its source, or blocked retries) or a rollout fails (render, validation, plan, apply, prune or health), and `False` with `RolledBack` while a completed rollback, manual or automatic, holds the desired revision. It keeps its last value, with no transition, while a rollout is planned, awaits approval or dependencies, or is progressing, while drift is reported without `selfHeal`, and while `spec.suspend` is set, so a Ready `True` Application may be drifted or suspended. Its `observedGeneration` is the generation it was computed for. |
| Application | `DependenciesReady`  | `True` with `DependenciesHealthy`; `False` with `DependencyNotReady` or `DependencyCycle`. Present only with `spec.dependsOn`.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| Application | `NotificationsReady` | `True` with `SinksReady`; `False` with `SinkInvalid` when a sink or its Secret is missing or invalid.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |
| Revision    | `RolloutComplete`    | `False` with `RollingOut` while hooks and waves apply; `True` once every group is Healthy.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| Revision    | `RolledBack`         | `True` on every Revision of the source revision a completed rollback replaced: `ManualRollback` for `ksync rollback`, `RollbackCompleted` for a failure policy. Kuvryn Sync does not deploy it again; see [Rollback](concepts.md#rollback) for how a hold ends.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        |
| ImagePolicy | `Ready`              | `True` with `Selected` and the selected image; `False` with `ScanFailed`.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                              |

## Failure reasons

`status.failure.reason` on a Revision, and the reason of a `False` `Ready`
condition on an Application, is one of these:

| Reason                                   | Meaning                                                                                                               |
| ---------------------------------------- | --------------------------------------------------------------------------------------------------------------------- |
| `SourceFailure`, `AuthenticationFailure` | The Repository is missing, or the Git source could not be fetched or authenticated.                                   |
| `ServiceAccountRequired`                 | Neither `spec.serviceAccountName` nor `--default-service-account` is set.                                             |
| `ServiceAccountFailure`                  | The service account could not be used.                                                                                |
| `DecryptionFailure`                      | The SOPS key Secret is missing, unlabelled, or unreadable.                                                            |
| `RenderFailure`                          | YAML, Kustomize or Helm rendering failed, including a Helm chart that could not be pulled.                            |
| `ValidationFailure`                      | Desired objects are unsafe or invalid, such as a duplicate object, an unknown hook, or a foreign namespace.           |
| `PlanFailure`                            | Live state could not be read or the plan could not be built.                                                          |
| `ConflictFailure`                        | A Server-Side Apply ownership conflict under `conflictPolicy: fail`.                                                  |
| `ApplyFailure`                           | The API server refused an apply.                                                                                      |
| `PruneFailure`                           | A managed resource could not be pruned, such as when the service account may not delete it.                           |
| `Forbidden`                              | The Application's service account may not read, apply, or delete a resource.                                          |
| `HealthFailure`, `HookFailed`            | A managed resource or hook is Degraded; `status.diagnosis` explains why.                                              |
| `TimeoutFailure`                         | Health was still Progressing when `spec.health.timeout` ran out.                                                      |
| `RollbackFailed`                         | Rollback was requested but no previous healthy Revision exists.                                                       |
| `RetryBlocked`                           | Retries stopped after `failurePolicy.maxAttempts`; the message keeps the last failure.                                |
| `RolledBack`                             | A completed rollback holds the desired revision; the Application runs the rollback target until a new commit arrives. |

## Events

Kuvryn Sync records Kubernetes Events on its own objects. Messages are redacted.

| Object      | Reason                                      | Type    | When                                                                                                                            |
| ----------- | ------------------------------------------- | ------- | ------------------------------------------------------------------------------------------------------------------------------- |
| Application | `PlanCreated`                               | Normal  | A Revision's plan was built.                                                                                                    |
| Application | `ApprovalRequired`                          | Normal  | The plan waits for manual approval.                                                                                             |
| Application | `ApprovalStale`                             | Warning | The plan changed after it was approved; approve again.                                                                          |
| Application | `DeploymentStarted`                         | Normal  | A rollout began applying.                                                                                                       |
| Application | `DeploymentHealthy`                         | Normal  | Every managed resource is Healthy.                                                                                              |
| Application | `Diagnosed`                                 | Warning | The root causes in `status.diagnosis` changed, or the Application became Degraded.                                              |
| Application | `RollbackStarted`, `RollbackCompleted`      | both    | A failure triggered rollback, and a manual or automatic rollback finished.                                                      |
| Application | `RollbackAbandoned`                         | Warning | A rollback's target failed for a reason retrying cannot fix, or used up its `maxAttempts`, so the request was removed.          |
| Application | `RollbackHoldLifted`                        | Normal  | An explicit rollback to a held Revision lifted its hold.                                                                        |
| Application | a [failure reason](#failure-reasons)        | Warning | A Revision failed, or retries stopped (`RetryBlocked`).                                                                         |
| Application | `PruneInventoryIncomplete`                  | Warning | The service account may not list some managed kinds, so they are not pruned.                                                    |
| Application | `PruneSkipped`                              | Warning | Prune kept opted-out or high-risk managed resources that desired state no longer declares; once per attempt, naming up to five. |
| Application | `ManagedResourcesOrphaned`, `Forbidden`     | Warning | Deleting with `DeleteManagedResources` left objects the service account could not delete.                                       |
| Application | `InvalidHealthCheck`                        | Warning | A HealthCheck rule for a managed kind is invalid.                                                                               |
| Application | `NotificationFailed`, `NotificationDropped` | Warning | A notification could not be delivered, or the queue was full.                                                                   |
| Repository  | `RepositoryReady`                           | Normal  | The source resolved.                                                                                                            |
| Repository  | the `Ready` condition's failure reason      | Warning | The source could not be resolved or discovery failed.                                                                           |
| Repository  | `ImagesUpdated`, `ImageUpdateFailed`        | both    | Image write-back committed a change, or failed.                                                                                 |
| ImagePolicy | `ImageSelected`                             | Normal  | A new image was selected.                                                                                                       |

Every Application Event is also counted in `kuvryn_sync_lifecycle_events_total`.

## Labels and annotations

| Key                                           | On                           | Set by             | Meaning                                                                                                                                                                             |
| --------------------------------------------- | ---------------------------- | ------------------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `sync.kuvryn.io/application`                  | managed objects, Revisions   | Kuvryn Sync        | Owning Application name; prune and drift find managed objects by it.                                                                                                                |
| `sync.kuvryn.io/application-namespace`        | managed objects              | Kuvryn Sync        | Owning Application namespace.                                                                                                                                                       |
| `sync.kuvryn.io/revision`                     | managed objects (annotation) | Kuvryn Sync        | Revision object that last applied it.                                                                                                                                               |
| `sync.kuvryn.io/prune: disabled`              | managed objects (annotation) | you                | Never prune this object: prune skips it and reports it with a `PruneSkipped` Event.                                                                                                 |
| `sync.kuvryn.io/hook`                         | desired objects (annotation) | you                | `pre-sync`, `post-sync`, or `skip`; see [Sync hooks and waves](operations.md#sync-hooks-and-waves).                                                                                 |
| `sync.kuvryn.io/sync-wave`                    | desired objects (annotation) | you                | Integer wave, default `0`.                                                                                                                                                          |
| `sync.kuvryn.io/repository`                   | discovered Applications      | Kuvryn Sync        | Repository that discovered the Application.                                                                                                                                         |
| `sync.kuvryn.io/discovered-from`              | discovered Applications      | Kuvryn Sync        | `.ksync.yaml` path the Application came from.                                                                                                                                       |
| `sync.kuvryn.io/approved-revision`            | Application (annotation)     | you or CLI         | Revision approved for a manual sync; see [Manual approval](operations.md#manual-approval).                                                                                          |
| `sync.kuvryn.io/approve-digest`               | Application (annotation)     | you or CLI         | Plan digest the approval is for; checked and never stored.                                                                                                                          |
| `sync.kuvryn.io/approved-by`                  | Application (annotation)     | webhook            | Who approved; part of the audit record, which cannot be set by hand.                                                                                                                |
| `sync.kuvryn.io/approved-at`                  | Application (annotation)     | webhook            | When the approval was recorded.                                                                                                                                                     |
| `sync.kuvryn.io/approved-digest`              | Application (annotation)     | webhook            | Plan digest the approval covers.                                                                                                                                                    |
| `sync.kuvryn.io/rollback-revision`            | Application (annotation)     | CLI or Kuvryn Sync | Source revision to roll back to; set by `ksync rollback` or a `rollback` failure policy, and removed once the rollback completes or is abandoned.                                   |
| `sync.kuvryn.io/rollback-from`                | Application (annotation)     | CLI or Kuvryn Sync | Source revision rolled back from, held once the rollback completes. When missing, Kuvryn Sync records the commit the spec resolves to; a new request while one is pending keeps it. |
| `sync.kuvryn.io/rollback-kind`                | Application (annotation)     | CLI or Kuvryn Sync | `manual` or `automatic`; missing means `manual`.                                                                                                                                    |
| `sync.kuvryn.io/reconcile-requested-at`       | Repository (annotation)      | receiver           | Requests an immediate fetch; set by the push webhook receiver.                                                                                                                      |
| `sync.kuvryn.io/git-credentials: "true"`      | Secret                       | you                | Allows the Secret as Git credentials.                                                                                                                                               |
| `sync.kuvryn.io/registry-credentials: "true"` | Secret                       | you                | Allows the Secret as registry credentials for charts and ImagePolicies.                                                                                                             |
| `sync.kuvryn.io/decryption-key: "true"`       | Secret                       | you                | Allows the Secret as SOPS age keys.                                                                                                                                                 |

High-risk kinds (Namespaces, CustomResourceDefinitions, PersistentVolumeClaims,
PersistentVolumes, and Secrets) are never pruned during a sync, and neither is
an object annotated `sync.kuvryn.io/prune: disabled`. With `spec.sync.prune`
enabled, removing one from desired state does not fail the rollout: prune
skips it and deletes the rest, the Revision plan lists it as `Unchanged` with
a warning saying why, and a `PruneSkipped` Warning Event names it once per
attempt. It keeps Kuvryn Sync's labels, so it stays in the inventory. Delete such
an object by hand once it is no longer needed.

## Invariants

- Sync state and health state are separate.
- Revision status stores bounded plan summaries and redacted per-resource changes.
- Server-Side Apply conflicts fail by default.
- Secret values must not appear in status, CLI output, Events, logs, metrics, or diagnostics.
- Rollback uses normal plan/apply/observe machinery and points at a previous healthy Revision.
