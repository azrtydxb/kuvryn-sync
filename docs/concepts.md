---
title: Concepts
nav_order: 3
---

# Concepts

## Repository

A `Repository` describes a desired-state source. In `v1alpha1`, the source type
is Git. Kuvryn Sync resolves a branch, tag, or commit to an observed revision and
records source readiness in status.

Private Git authentication is referenced through Kubernetes Secrets. The API
stores references to credentials, never the credential values.

After resolving a Git revision, the Repository controller looks for configured
`.ksync.yaml` files. By default it reads the repository root `.ksync.yaml`; for
monorepos, `spec.applicationConfigPaths` can point at one or more nested
`.ksync.yaml` files. These files are the GitOps entry point for Application
definitions: Kuvryn Sync creates, updates, and removes Applications that are managed
by that Repository label.

Each configured path must be repository-relative, must stay inside the checkout,
and must be named `.ksync.yaml`. Each file can contain one `Application` object
or an `applications:` list. Application names must be unique across all files, and
discovered Applications are annotated with `sync.kuvryn.io/discovered-from` so
operators can see which Git config file owns them.

## Application

An `Application` describes a deployable unit:

- source Repository, revision, path, and renderer;
- the service account Kuvryn Sync acts as when it reads, applies, and prunes;
- destination namespace constraints;
- sync policy for automatic apply, pruning, self-heal, and conflict handling;
- health observation timeout;
- bounded Revision history;
- optional rollback-on-failure behavior.

Applications are the primary object operators watch with `kubectl get app` or
`ksync apps`. They can be applied directly to the Kubernetes API, or declared
in the source repository's `.ksync.yaml` files for Repository-driven GitOps
bootstrapping.

Applications can depend on other Applications in the same namespace with
`spec.dependsOn`, for example workloads on the operator that serves their
custom resources. Kuvryn Sync still plans a dependent, but applies it only once
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

Kuvryn Sync keeps convergence and operational health separate:

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

Health keeps being observed after a rollout finishes, including while drift is
reported without `selfHeal`. When a workload stops being available while
nothing changes in Git, such as a Pod that crash-loops or is OOMKilled, the
Application turns Degraded with a [diagnosis](#resource-graph-and-diagnosis)
and a `HealthDegraded` Event. A managed resource that was deleted is Degraded
too. The Application turns Progressing instead while a resource is only on its
way, such as a Deployment scaling up, and no cause is evident, and Healthy
again once every resource is. Changes to Deployments, StatefulSets and
DaemonSets are noticed as they happen; everything is checked again at the
drift resync interval, five minutes by default.

Observing a degradation changes no Revision, sends no notification and does not
trigger the failure policy. Dependents wait while the Application is not
Healthy. With `selfHeal`, a drift repair is a rollout again: it can fail on the
same degradation, and the failure policy then acts on it.

## Resource graph and diagnosis

When a managed resource is not Healthy, Kuvryn Sync builds a graph of the live
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
how a missing Secret becomes a root cause. Kinds Kuvryn Sync does not know only
contribute their `ownerReferences`; they never fail reconciliation.

Diagnosis starts from each unhealthy managed resource and prefers the most
specific evidence: a container waiting to start (`ImagePullBackOff`,
`CrashLoopBackOff` with its last exit code, `CreateContainerConfigError`), an
unschedulable Pod, a Pending claim, a missing ConfigMap or Secret, a Service
without ready endpoints, or a failed Job. The result is
[`status.diagnosis`](api.md#diagnosis); `ksync graph` prints the graph itself.

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

Kuvryn Sync applies with Kubernetes Server-Side Apply. The default conflict policy,
`fail`, blocks ownership conflicts instead of force-taking fields; `adopt`
takes ownership of conflicting fields and lists each one, with its previous
manager, in the plan.

When pruning is enabled, Kuvryn Sync deletes previously managed resources that are no
longer present in desired state. It finds them by label across every kind in
the Application's `status.managedKinds` inventory, so objects of any kind are
pruned, including after a controller restart. Destructive changes are
represented in the plan before mutation.

Prune skips, and never deletes, two kinds of managed resources:

- resources annotated `sync.kuvryn.io/prune: "disabled"`, a per-resource opt-out;
- high-risk kinds, whose deletion loses data or other workloads' state:
  Namespaces, CustomResourceDefinitions, PersistentVolumeClaims,
  PersistentVolumes and Secrets.

Skipping is not a failure: the rest of the prune proceeds and the rollout
completes. The plan lists each skipped resource as `Unchanged` with a warning
saying why, and a `PruneSkipped` Warning Event names them. Skipped resources
keep Kuvryn Sync's labels and stay in the inventory, so the plan shows them on
every Revision without trying to delete them again, and putting one back in
Git adopts it as before. Delete one by hand once it is no longer needed. With
`deletionPolicy: DeleteManagedResources`, deleting the Application still
deletes high-risk resources, and still keeps resources that opted out.

## Drift and self-heal

Kuvryn Sync can detect live drift by comparing normalized live state to desired state.
When `selfHeal` is enabled, drift is corrected through the same plan/apply path.
When it is off, drift is only reported (`Drifted`): the edit is left in place,
even when it took over a field Kuvryn Sync manages, the Revision stays Healthy, and
undoing the edit returns the Application to Synced without a new rollout.

Kuvryn Sync notices drift immediately for kinds it watches: ConfigMaps, Secrets,
Services, Deployments, StatefulSets, and DaemonSets, plus any managed kind the
controller has been granted `list` and `watch` on. Watches are metadata-only.
Applications that manage other kinds are re-checked every
`--drift-resync-interval` (Helm value `driftResyncInterval`, default 5m). See
[Operations](operations.md#drift-detection-for-other-kinds) to grant watches.

## Rollback

A rollback records its intent on the Application: the source revision to roll
back to, the source revision it rolls back from, and whether it is manual
(`ksync rollback`) or automatic (a `rollback` failure policy). A request
that does not say what it rolls back from rolls back from the commit the
Application's spec resolves to, and a second `ksync rollback` while one is
pending keeps the first one's source. Kuvryn Sync then
runs normal reconciliation against the target, with the same validation,
planning, apply, health, and event behavior as a forward sync.

On an Application with manual sync (`spec.sync.automatic: false`), a manual
rollback is its own approval of the Revision you chose: asking to roll back to
it is the decision to deploy it, so no separate `ksync sync` is needed.
`ksync rollback` names that Revision object in
`sync.kuvryn.io/rollback-target-revision`. The admission webhook refuses a
Revision that does not exist, belongs to another Application or is of another
commit, and records, from the authenticated request, who asked
(`sync.kuvryn.io/rollback-requested-by`), when
(`sync.kuvryn.io/rollback-requested-at`) and what the rollback approves
(`sync.kuvryn.io/rollback-target-hash`). It records them again whenever the
target, the chosen Revision or the kind changes, and restores them on every
other change.

`rollback-target-hash` fingerprints the desired state the Revision's last
completed, Healthy rollout deployed, or that Kuvryn Sync found already live
while reconciling it outside a rollback, or in an approved rollback (its
`status.deployedDesiredStateHash`),
together with the Application's `spec.sync` (prune, conflict policy,
self-heal) and `spec.strategy` at request time. It is not the Revision's
latest render: `spec.desiredStateHash` follows every render, including drift
checks that deploy nothing, so values changed after the Revision deployed
could otherwise be approved. A Revision with no recorded deployed state gets
no `rollback-target-hash`, and a rollback to it waits for `ksync sync`: one
no completed rollout deployed, or one deployed before Kuvryn Sync 0.6.4 that
has not been reconciled as the live Revision since. A pending rollback never
records it without approval. The
field needs the Revision CRD from 0.6.4: re-apply the CRDs when upgrading
(the Helm chart does not ship them). Until then the API server drops it, and
every manual rollback waits for `ksync sync`, which `ksync rollback` reports
as a Revision that never deployed.

The approval is bound to that Revision, to the desired state it deployed,
and to the sync policy and strategy at request time. Kuvryn Sync re-renders
the target and approves it only if the Revision it builds is the one you
chose, it renders that desired state, and `spec.sync` and `spec.strategy` are
unchanged. What it approves is the plan for that desired state against the
live state as it is then, under that sync policy: the plan itself is computed
at rollback time and not shown to you first. It records the approval on the
Revision's `status.approval` under your user and request time, for the digest
of the plan it applies in that reconcile, with a `RollbackApproved` Event.
Once the rollout starts, the approval covers the rest of it, as a manual
approval does, but only while the desired state stays the one you chose.

If the target changed after you asked, the request does not approve it, and
the Revision waits in `AwaitingApproval` with an `ApprovalStale` Event and a
`RollbackTargetChanged` Warning Event. That happens when the Application's
`spec.source.path`, `spec.source.render` or service account changed, so Kuvryn
Sync builds another Revision of the commit; when the chosen Revision renders
differently from what it deployed, for example after a Helm `valuesFrom`
Secret changed; or when `spec.sync` or `spec.strategy` changed.
Review the new plan with `ksync plan` and approve it with `ksync sync`; the
rollback request stays until it completes or is abandoned.

While a rollback is requested on an Application with manual sync, the only
approvals that deploy it are the manual request's own, as above, and a
`ksync sync` given at or after the request (`rollback-requested-at`). An
approval recorded before the request, even of the same plan, does not count,
so it cannot deploy the rollback past the chosen Revision, requester and
`rollback-target-hash`. A request without a recorded time, from before 0.6.4
or written with webhooks disabled, accepts any approval of the current plan.
A rollback whose target is already live, which changes nothing, still needs
one of these approvals: completing it holds the replaced commit and records
the target's desired state as deployed.

Kuvryn Sync approves a rollback target this way only for a manual rollback
with a recorded requester and chosen Revision. A rollback a failure policy
starts on an Application with manual sync waits for approval, and so does a
request set by hand without `rollback-target-revision`. Automatic Applications
need no approval, so for them nothing changes.

With webhooks disabled (`ENABLE_WEBHOOKS=false`), nothing verifies these
annotations: anyone who can update the Application can write
`rollback-requested-by`, `rollback-target-hash` and the rest by hand, and
Kuvryn Sync trusts them and deploys the target under the name written there,
as it trusts forged approval annotations; see
[Manual approval](operations.md#manual-approval). Turning webhooks on again
does not clear a record written meanwhile, since the webhook keeps an
unchanged request's record. At startup with webhooks enabled, the manager logs
every Application carrying a pending rollback request with a requester; if one
may have been written while webhooks were off, remove its
`sync.kuvryn.io/rollback-*` annotations and request the rollback again.
Requests are not dropped by age, because a legitimate rollback whose rollout
spans a manager restart, such as an upgrade, would then lose its approval.

Kuvryn Sync takes the approval from the rollback rather than having
`ksync rollback` wait for the new plan and approve its digest: a digest the
CLI approved could go stale after a failed attempt is retried and leave the
rollback waiting again. A rollback therefore deploys the chosen Revision
without showing you its new plan against the live state first; the plan it
applied stays on the Revision, and `ksync history` names who approved it.

When the rollback completes, it holds. Every Revision of the source revision
rolled back from, in any phase, is marked `Failed` with a `RolledBack`
condition (reason `ManualRollback` or `RollbackCompleted`), and Kuvryn Sync does not
deploy it again. The Application keeps running the target: Kuvryn Sync still
reconciles it, observing its health, reporting drift of the target as
`Drifted`, and self-healing when `spec.sync.selfHeal` is set. Otherwise sync is
`OutOfSync`, since the held desired revision is not deployed, and `Ready` is
`False` with reason `RolledBack`. An approval of a held Revision is ignored,
and history retention never deletes a held Revision, which does not count
against `spec.history.limit`.

The hold is keyed on the commit and the Revision identity. It ends when:

- a new commit arrives, creating a new Revision;
- `spec.source.path`, `spec.source.render` or the service account changes,
  which also creates a new Revision for the same commit: a changed spec is new
  desired state, so it deploys;
- you delete the held Revision, which Kuvryn Sync then creates afresh;
- you roll back to the held Revision explicitly, with
  `ksync rollback --revision`, which lifts its hold and deploys it;
- the held commit is the one deployed, as after an identity change that
  deployed it is reverted: the old Revision is lifted and reconciled normally.

A change that keeps the Revision identity, such as a new value in a Secret
named by Helm `valuesFrom`, does not end the hold.

A rollback that cannot reach its target is abandoned: Kuvryn Sync removes the
request, emits a `RollbackAbandoned` Warning Event, and reconciles the desired
revision again. That happens when the target fails for a reason retrying cannot
fix, such as invalid desired state, or once its `maxAttempts` are used up. A
failed fetch of the target, or a retryable failure such as a failed chart pull,
keeps the request, and Kuvryn Sync tries the target again after its backoff. To give
up such a rollback yourself, remove the `sync.kuvryn.io/rollback-*` annotations.

## Events, metrics, and tracing

Kuvryn Sync emits Kubernetes Events for lifecycle transitions, registers Prometheus
collectors with bounded labels, and exports OpenTelemetry traces over OTLP when
an endpoint is configured.
Public integrations should prefer CRDs, Conditions, Events, and metrics over
controller internals.
