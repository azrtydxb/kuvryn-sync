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

Applications can depend on other Applications in the same namespace with
`spec.dependsOn`, for example workloads on the operator that serves their
custom resources. Solder still plans a dependent, but applies it only once
every dependency is Healthy at the revision it currently wants, and reports
what it waits for in the `DependenciesReady` condition. Dependents are
re-queued as soon as a dependency changes.

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

Applications can order their rollout with
[sync hooks and waves](operations.md#sync-hooks-and-waves); each group must be
Healthy before the next is applied.

A [HealthCheck](api.md#healthcheck) overrides these rules for one kind with CEL
expressions. A rollout waits only for Progressing resources, until
`spec.health.timeout`.

## Resource graph and diagnosis

When a managed resource is not Healthy, Solder builds a graph of the live
objects around the Application's managed resources and walks it down to the
evidence that explains the failure. The graph is deterministic and has these
edges:

| Edge        | From                             | To                                                                                                                              |
| ----------- | -------------------------------- | ------------------------------------------------------------------------------------------------------------------------------- |
| `Owns`      | an owner                         | each object whose `ownerReferences` name it: a Deployment its ReplicaSets, a ReplicaSet or Job its Pods                         |
| `Selects`   | a Service or PodDisruptionBudget | the Pods its selector matches; for a Service also the workloads whose Pod template it matches                                   |
| `Endpoints` | a Service                        | EndpointSlices labelled `kubernetes.io/service-name`                                                                            |
| `Routes`    | an Ingress                       | the Services of its rules and default backend                                                                                   |
| `Scales`    | a HorizontalPodAutoscaler        | its `scaleTargetRef`                                                                                                            |
| `Binds`     | a PersistentVolumeClaim          | the PersistentVolume in `spec.volumeName`                                                                                       |
| `Mounts`    | a Pod or workload                | the claims its volumes use                                                                                                      |
| `Uses`      | a Pod or workload                | ConfigMaps and Secrets from `envFrom`, `env` value sources, `configMap`, `secret` and projected volumes, and `imagePullSecrets` |
| `RunsAs`    | a Pod or workload                | its ServiceAccount                                                                                                              |

An object that is referenced but does not exist is a `missing` node, which is
how a missing Secret becomes a root cause. Kinds Solder does not know only
contribute their `ownerReferences`; they never fail reconciliation.

Diagnosis starts from each unhealthy managed resource and prefers the most
specific evidence: a container waiting to start (`ImagePullBackOff`,
`CrashLoopBackOff` with its last exit code, `CreateContainerConfigError`), an
unschedulable Pod, a Pending claim, a missing ConfigMap or Secret, a Service
without ready endpoints, or a failed Job. The result is
[`status.diagnosis`](api.md#diagnosis); `solder graph` prints the graph itself.

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

Solder applies with Kubernetes Server-Side Apply. The default conflict policy,
`fail`, blocks ownership conflicts instead of force-taking fields; `adopt`
takes ownership of conflicting fields and lists each one, with its previous
manager, in the plan.

When pruning is enabled, Solder deletes previously managed resources that are no
longer present in desired state. It finds them by label across every kind in
the Application's `status.managedKinds` inventory, so objects of any kind are
pruned, including after a controller restart. Destructive changes are
represented in the plan before mutation.

## Drift and self-heal

Solder can detect live drift by comparing normalized live state to desired state.
When `selfHeal` is enabled, drift is corrected through the same plan/apply path.
When it is off, drift is only reported (`Drifted`): the edit is left in place,
even when it took over a field Solder manages, the Revision stays Healthy, and
undoing the edit returns the Application to Synced without a new rollout.

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
collectors with bounded labels, and exports OpenTelemetry traces over OTLP when
an endpoint is configured.
Public integrations should prefer CRDs, Conditions, Events, and metrics over
controller internals.
