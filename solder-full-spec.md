# Solder — Full Product Concept & Engineering Specification

> **Solder — GitOps that sticks.**
>
> Lightweight, deterministic, Kubernetes-native GitOps built in Go.

**Status:** Initial product and implementation specification  
**Audience:** AI coding agent / engineering team  
**Initial API:** `v1alpha1`

---

## 1. Vision

Solder is a lightweight Kubernetes-native GitOps deployment and reconciliation engine intended as a modern alternative to heavier or fragmented GitOps stacks.

Solder is **not “Argo CD rewritten.”** It rethinks GitOps around a small Go operator, Kubernetes-native state, Server-Side Apply, first-class deployment plans, dependency-aware health, deterministic rollback, and clean integration boundaries.

The name is literal: solder sticks things together. Solder joins desired state in Git to actual state in Kubernetes and continuously keeps that connection intact.

```text
 Any CI system or person
       |
       | promote desired state
       v
      Git
       |
       v
+-----------------------+
|        SOLDER         |
|-----------------------|
| Source                |
| Render                |
| Plan / Diff           |
| Reconcile             |
| Apply                 |
| Observe               |
| Health                |
| Drift / Self-Heal     |
| Revision / Rollback   |
+-----------+-----------+
            |
            v
       Kubernetes
```

The boundary is deliberate: **Solder** owns desired state -> running state -> continuous reconciliation. Building artifacts, promoting desired state, and visualizing or operating clusters belong to other tools, which integrate with Solder only through its public interfaces (see section 23).

Solder MUST be universal: useful on its own and with any CI system, UI or operations tool, and never built for one specific product.

---

## 2. Core Design Principles

1. **Operator first. CLI second. UI elsewhere.**
2. One small Go controller, not a collection of mandatory microservices.
3. Kubernetes is the primary operational database.
4. No mandatory Redis, PostgreSQL, message broker, repo server, UI server or SSO service.
5. Pull-based reconciliation. CI should not need broad Kubernetes deployment credentials.
6. Server-Side Apply (SSA) is the primary apply mechanism.
7. Sync state and health state are independent.
8. Every deployment attempt produces an auditable Revision.
9. Every mutation can be preceded by a deterministic Change Plan.
10. Drift detection and optional self-healing are core features.
11. Kubernetes relationships form an application dependency graph.
12. Failure handling and rollback are first-class.
13. AI is optional and never participates in correctness-critical reconciliation.
14. Design product-neutral integration surfaces from the beginning: public CRDs, status, Events, metrics and CLI.
15. Avoid CRD sprawl.
16. Prefer immutable artifact digests.
17. Default to conservative, explainable behavior over magic.
18. Keep status and history bounded so Solder does not abuse etcd.

---

## 3. Goals and Non-Goals

### Goals

Solder should provide:

- simple operator installation;
- Git desired-state sources;
- Git HTTPS and SSH authentication through Kubernetes Secrets;
- plain YAML, Kustomize and Helm rendering;
- source polling and event-driven reconciliation where available;
- deterministic desired/live diff;
- human- and machine-readable deployment plans;
- automatic and manual synchronization;
- Server-Side Apply;
- safe pruning;
- drift detection;
- optional self-healing;
- application and resource health evaluation;
- dependency-aware apply/prune ordering;
- deployment history;
- deterministic rollback;
- Kubernetes Events;
- Prometheus-compatible metrics;
- structured logging;
- CLI;
- optional OpenTelemetry;
- stable, product-neutral integration surfaces for any tool.

### Initial non-goals

Do NOT initially build:

- mandatory web UI;
- internal user/account database;
- embedded SSO;
- Redis;
- PostgreSQL;
- message broker;
- separate repository microservice;
- CI pipeline engine;
- container builder;
- artifact registry;
- Git server;
- service mesh;
- AI that directly mutates desired state;
- full fleet-management platform;
- dozens of CRDs.

---

## 4. Installation and Technology

Default installation:

```text
Namespace/solder-system
Deployment/solder-controller
ServiceAccount/solder-controller
ClusterRole
ClusterRoleBinding

CRDs:
  repositories.<api-group>
  applications.<api-group>
  revisions.<api-group>
```

Binaries:

```text
solder-controller
solder
```

Controller implementation:

- Go
- Kubernetes controller-runtime
- Kubernetes API machinery
- structured Go logging
- Prometheus-compatible metrics
- optional OpenTelemetry

Initial development API may use a placeholder such as:

```text
solder.io/v1alpha1
```

The real API domain MUST be verified before public release.

Rendering adapters normalize all desired state to:

```go
[]unstructured.Unstructured
```

Conceptual interface:

```go
type Renderer interface {
    Render(ctx context.Context, input RenderInput) ([]unstructured.Unstructured, error)
}
```

The reconciliation engine MUST NOT care which renderer produced the resources.

---

## 5. Public API and Core CRDs

Keep the initial public API deliberately small:

1. `Repository`
2. `Application`
3. `Revision`

Do not create CRDs merely because an internal concept exists.

### 5.1 Repository

A Repository represents a desired-state source and its authentication.

```yaml
apiVersion: solder.io/v1alpha1
kind: Repository
metadata:
  name: platform
  namespace: solder-system
spec:
  type: git
  git:
    url: ssh://git@github.com/acme/platform.git
    auth:
      secretRef:
        name: platform-git
  pollInterval: 60s

status:
  state: Ready
  observedRevision: 8c51af2
  lastFetchedAt: "2026-09-17T06:00:00Z"
  conditions:
    - type: Ready
      status: "True"
      reason: FetchSucceeded
```

Initial Repository requirements:

- Git HTTPS;
- Git SSH;
- branches, tags and exact commits;
- Kubernetes Secret references;
- configurable polling;
- readiness and observed revision;
- errors surfaced via Conditions and Events;
- credentials never exposed in logs/status/Events.

Future source adapters may include OCI and Helm repositories without changing the reconciliation core.

### 5.2 Application

Application is the main user-facing deployment abstraction.

```yaml
apiVersion: solder.io/v1alpha1
kind: Application
metadata:
  name: payments
  namespace: solder-system
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

  health:
    timeout: 5m

  history:
    limit: 20

status:
  state: Healthy
  desiredRevision: 8c51af2
  deployedRevision: 8c51af2

  sync:
    state: Synced

  health:
    state: Healthy

  resources:
    total: 12
    healthy: 12
    progressing: 0
    degraded: 0
    unknown: 0

  conditions: []
```

Suggested sync states:

```text
Unknown
Synced
OutOfSync
Drifted
Planning
AwaitingApproval
Applying
Pruning
```

Suggested health states:

```text
Unknown
Progressing
Healthy
Degraded
Suspended
```

These MUST remain separate. An Application can be `Synced + Degraded` or `Drifted + Healthy`.

### 5.3 Rendering

Plain YAML:

```yaml
render:
  type: yaml
```

Kustomize:

```yaml
render:
  type: kustomize
```

Helm:

```yaml
render:
  type: helm
  helm:
    releaseName: payments
    valuesFiles:
      - values-production.yaml
```

Do not unnecessarily recreate Helm or Kustomize functionality. Solder's responsibility is to safely obtain normalized Kubernetes objects.

### 5.4 Revision

Every attempted deployment creates a Revision.

```yaml
apiVersion: solder.io/v1alpha1
kind: Revision
metadata:
  name: payments-8c51af2
  namespace: solder-system
spec:
  applicationRef:
    name: payments

  source:
    revision: 8c51af2

status:
  phase: Healthy
  startedAt: "..."
  completedAt: "..."

  plan:
    create: 2
    update: 4
    delete: 1
    unchanged: 8

  health:
    healthy: 14
    degraded: 0

  previousRevision:
    name: payments-7aa9231
```

Revision lifecycle:

```text
Pending
Planning
AwaitingApproval
Applying
Observing
Healthy
Failed
RollingBack
RolledBack
Cancelled
```

Revision spec is immutable after creation; status progresses through the lifecycle.

History MUST be bounded. Example:

```yaml
history:
  limit: 20
```

Old Revisions are garbage-collected according to policy. Do not store unlimited manifests/diffs in etcd.

---

## 6. Change Plan

Change Plan is first-class but initially stored within Revision rather than as another CRD.

```yaml
status:
  plan:
    summary:
      create: 1
      update: 1
      delete: 1
      unchanged: 11

    resources:
      - resource:
          apiVersion: apps/v1
          kind: Deployment
          namespace: payments
          name: api
        action: Update
        changes:
          - path: spec.replicas
            before: 3
            after: 5
          - path: spec.template.spec.containers[0].image
            before: payments@sha256:old
            after: payments@sha256:new
```

CLI:

```text
$ solder plan payments

Application: payments
Revision:    8c51af2

~ Deployment/payments/api

    replicas
      3 -> 5

    containers[api].image
      sha256:OLD -> sha256:NEW

+ ConfigMap/payments/runtime
- ConfigMap/payments/legacy

2 changed
1 created
1 deleted
11 unchanged
```

Planner requirements:

- normalize desired/live resources;
- ignore expected server-generated data;
- account for SSA ownership;
- classify create/update/delete/unchanged;
- identify conflicts;
- deterministic output;
- Secret values always redacted;
- bounded output size;
- deletion visually and structurally conspicuous;
- machine-readable representation.

---

## 7. Reconciliation Engine

```text
Application event / source poll
        |
        v
Resolve Repository
        |
        v
Resolve source revision
        |
        v
Fetch source
        |
        v
Render
        |
        v
Normalize + Validate
        |
        v
Build Resource Graph
        |
        v
Load Live State
        |
        v
Compute Diff
        |
        v
Create Revision + Plan
        |
        +---- no changes ----> Synced / Observe Health
        |
        v
Evaluate Sync Policy
        |
        +---- manual ----> AwaitingApproval
        |
        v
Order Dependency Groups
        |
        v
Server-Side Apply
        |
        v
Prune Eligible Resources
        |
        v
Observe Rollout
        |
        v
Evaluate Health
        |
    +---+---+
    |       |
 Healthy   Failed
    |       |
 Complete  Deterministic Diagnosis
            |
            v
       Failure Policy
        |        |
       Pause   Rollback
```

The reconciler MUST be idempotent, restart-safe, duplicate-event-safe, context/deadline bounded, rate-limited, backoff-aware and observable.

---

## 8. Server-Side Apply and Ownership

Solder uses Kubernetes Server-Side Apply with a stable field manager such as:

```text
solder
```

Managed resources receive discovery metadata:

```yaml
metadata:
  labels:
    solder.io/application: payments
  annotations:
    solder.io/revision: 8c51af2
```

Labels and annotations aid discovery; managed fields provide field ownership semantics.

SSA conflicts MUST be explicit. Default:

```yaml
sync:
  conflictPolicy: fail
```

Never silently force ownership. A future force mode must be explicit and auditable.

---

## 9. Pruning and Drift

With:

```yaml
sync:
  prune: true
```

Solder may delete objects that were previously managed by the Application, no longer exist in desired state, and pass pruning safety policy.

Per-resource opt-out:

```yaml
metadata:
  annotations:
    solder.io/prune: "disabled"
```

High-risk resources require conservative handling. Deletion should follow reverse dependency order where practical.

Drift example:

```text
Desired replicas: 5
Live replicas:    20
=> Drifted
```

With:

```yaml
sync:
  selfHeal: true
```

Solder reconciles managed drift. With `false`, it reports drift without mutation.

The drift engine MUST distinguish meaningful desired/live differences from normal mutations produced by Kubernetes and cooperating controllers.

---

## 10. Resource Dependency Graph

Solder builds an application-level resource graph.

Common inferable relationships include:

```text
Deployment
  -> ReplicaSet
     -> Pod

Deployment
  -> ServiceAccount
  -> ConfigMap
  -> Secret
  -> PVC

Ingress
  -> Service

Service
  -> EndpointSlice / selected Pods

HPA
  -> Deployment

PDB
  -> selected Pods

PVC
  -> PV
```

Conceptual model:

```go
type ResourceID struct {
    Group     string
    Version   string
    Kind      string
    Namespace string
    Name      string
}

type EdgeType string

type Edge struct {
    From ResourceID
    To   ResourceID
    Type EdgeType
}

type ResourceGraph struct {
    Nodes map[ResourceID]*Node
    Edges []Edge
}
```

The graph powers:

1. visualization;
2. apply ordering;
3. prune ordering;
4. health propagation;
5. deterministic root-cause analysis;
6. external tools, through the public API;
7. optional AI explanation.

Graph inference MUST tolerate incomplete relationships and unknown/custom resources.

---

## 11. Health and Diagnosis Engine

Health is more than resource existence.

Initial built-in evaluators should include:

- Deployment;
- StatefulSet;
- DaemonSet;
- Job;
- Pod;
- PVC;
- Service;
- Ingress;
- common autoscaling resources.

Example structured diagnosis:

```text
Application payments is Degraded

Deployment/payments-api unavailable
  -> ReplicaSet/payments-api-789d
     -> Pod/payments-api-789d-x29ds
        -> CreateContainerConfigError
           -> Secret/payments-db-v2 missing
```

Health output MUST be structured so the same result can drive:

- Application status;
- CLI output;
- Kubernetes Events;
- external visualization tools;
- optional AI explanation.

Future custom health rules may be declarative. Do not embed an unrestricted scripting runtime in the initial version.

---

## 12. Apply Ordering

Use dependency graph information plus Kubernetes semantics to form apply groups.

A baseline might resemble:

```text
Phase 1
  Namespace
  CRDs where applicable

Phase 2
  ServiceAccounts
  ConfigMaps
  Secrets
  PVCs

Phase 3
  Workloads

Phase 4
  Services

Phase 5
  Ingress / routing / dependent resources
```

Do not rely solely on a hard-coded Kind list. Explicit dependency edges should influence ordering.

CRDs and newly introduced custom resources require discovery refresh/retry handling.

---

## 13. Rollback

Rollback uses the same plan/apply machinery as forward reconciliation.

Do not implement rollback as an opaque command.

Example:

```text
ROLLBACK PLAN

Deployment/api
  image:
    sha256:NEW -> sha256:OLD

  replicas:
    5 -> 3

ConfigMap/runtime
  DELETE

ConfigMap/legacy
  RESTORE
```

Flow:

```text
Failure
  |
  v
Find last healthy Revision
  |
  v
Resolve its desired source state
  |
  v
Render
  |
  v
Plan current -> previous
  |
  v
Apply
  |
  v
Observe
  |
  v
RolledBack / RollbackFailed
```

A crucial semantic: Git may still request the failed version after cluster rollback. Solder MUST expose this honestly:

```text
Desired Revision:  bad123
Deployed Revision: good456
State: RolledBack / OutOfSync
```

It MUST NOT repeatedly enter an uncontrolled deploy-fail-rollback loop. Failed desired revisions need retry/backoff/suspension semantics until source changes or an operator explicitly retries.

Example policy:

```yaml
strategy:
  failurePolicy:
    action: rollback
    timeout: 5m
    maxAttempts: 2
```

---

## 14. Manual Approval

Applications may disable automatic synchronization:

```yaml
sync:
  automatic: false
```

Flow:

```text
new source revision
  -> render
  -> plan
  -> Revision/AwaitingApproval
```

CLI:

```text
solder plan payments
solder sync payments
```

Approval should identify the exact Revision being approved to prevent TOCTOU errors if Git changes between planning and approval.

---

## 15. CLI Specification

Core commands:

```text
solder version
solder install
solder status

solder repos
solder repo get <name>

solder apps
solder get <application>

solder plan <application>
solder sync <application>

solder history <application>
solder revision <revision>
solder rollback <application> [revision]

solder drift <application>
solder diagnose <application>

solder suspend <application>
solder resume <application>
```

Useful flags:

```text
-o table
-o yaml
-o json
--namespace
--context
--revision
--watch
--timeout
```

CLI should primarily interact with Kubernetes APIs/CRDs rather than requiring a proprietary always-on Solder API server.

---

## 16. Controller/Internal Go Structure

Recommended initial repository layout:

```text
cmd/
  solder-controller/
  solder/

api/
  v1alpha1/
    repository_types.go
    application_types.go
    revision_types.go
    groupversion_info.go
    zz_generated.deepcopy.go

internal/
  controller/
    repository/
    application/
    revision/

  source/
    source.go
    git/

  renderer/
    renderer.go
    yaml/
    kustomize/
    helm/

  normalize/
  validate/

  graph/
    graph.go
    inference/

  diff/
  planner/

  apply/
    ssa.go
    ordering.go
    prune.go

  drift/
  health/
    builtin/

  rollback/
  history/

  kube/
  events/
  metrics/
  telemetry/

pkg/
  client/
  types/

config/
  crd/
  rbac/
  manager/
  samples/

charts/
  solder/

docs/
```

Prefer internal packages until there is a clear reason to expose a stable Go library.

---

## 17. Controller Responsibilities

### Repository controller

Responsibilities:

- validate source configuration;
- resolve authentication Secret;
- fetch/inspect source;
- determine observed revision;
- update status;
- emit source-change signals;
- retry with backoff.

### Application controller

Responsibilities:

- resolve Repository;
- render desired state;
- normalize/validate;
- construct graph;
- obtain live state;
- plan;
- create Revision;
- apply according to policy;
- prune;
- observe health;
- detect drift;
- invoke rollback policy;
- update Application status.

### Revision handling

A separate controller is optional. Do not create one unless lifecycle complexity warrants it. Revision status may initially be driven by Application reconciliation.

---

## 18. Concurrency and Scaling

Solder should support many Applications without turning every source poll into expensive full-cluster scans.

Design requirements:

- controller-runtime work queues;
- configurable worker concurrency;
- per-Application reconciliation serialization;
- deduplicate bursts;
- exponential retry/backoff;
- cached Kubernetes watches;
- indexed lookup of managed resources;
- source cache;
- bounded Git checkout/cache size;
- jittered polling;
- avoid polling every Repository simultaneously;
- avoid full render/diff if source revision has not changed unless drift/health requires it.

Applications should be independent failure domains: one broken repo/render must not stall others.

---

## 19. Source Cache

Solder does not need a separate repo-server, but it DOES need an efficient local source cache.

Requirements:

- cache Git repositories in controller ephemeral storage;
- fetch deltas rather than recloning;
- key cache by canonical source identity;
- multiple Applications referencing one Repository reuse the same local data;
- serialize/coordinate concurrent fetches;
- configurable cache size;
- safe eviction;
- no credentials persisted unnecessarily;
- cache loss must only cause refetching, never correctness loss.

Future deployments may optionally use persistent cache storage, but it must not be required.

---

## 20. Security Model

Principles:

- least privilege;
- credentials only through Secrets;
- never log secret values;
- redact Secret diffs;
- validate paths and source inputs;
- protect against path traversal;
- avoid shelling out where a safe library is available;
- verify host keys/TLS according to explicit policy;
- restrict target namespaces when configured;
- clear RBAC modes for namespace-scoped vs cluster-scoped installations;
- audit manual sync, rollback, force/conflict operations;
- validate manifests before apply;
- bounded decompression/file sizes for source inputs.

Solder itself should not become a secret-management system.

Support references to normal Kubernetes Secrets and later integrate cleanly with external-secret/sealed-secret ecosystems.

---

## 21. Observability

### Structured logs

Every important log should include relevant identifiers:

```text
application
repository
revision
namespace
resource
reconcile_id
```

Never log secret payloads.

### Metrics

Initial metric families should cover:

```text
solder_reconcile_total
solder_reconcile_duration_seconds
solder_reconcile_errors_total

solder_applications
solder_application_health
solder_application_sync_state

solder_source_fetch_total
solder_source_fetch_duration_seconds
solder_source_errors_total

solder_plan_resources
solder_apply_total
solder_apply_errors_total

solder_drift_detected_total
solder_drift_reconciled_total

solder_rollbacks_total
solder_rollback_errors_total
```

Keep metric labels bounded; never put arbitrary commit IDs/resource names into high-cardinality labels without careful design.

### Kubernetes Events

Emit concise Events for meaningful lifecycle changes, not every internal loop.

### OpenTelemetry

Design tracing boundaries now:

```text
reconcile
  -> source.resolve
  -> source.fetch
  -> render
  -> graph
  -> diff
  -> plan
  -> apply
  -> health
  -> rollback
```

OTel export can remain optional.

---

## 22. Event Model

Solder should expose stable structured lifecycle events from the start.

Canonical event types:

```text
RepositoryReady
RepositoryFailed
RevisionDetected
PlanCreated
ApprovalRequired
DeploymentStarted
ResourceApplied
PruneStarted
ResourcePruned
HealthCheckStarted
DeploymentHealthy
DeploymentFailed
RollbackStarted
RollbackCompleted
RollbackFailed
DriftDetected
DriftReconciled
ApplicationSuspended
ApplicationResumed
```

Initially these map to CRD status, Kubernetes Events and internal typed events.

Later consumers may include:

```text
              Solder Events
                   |
        +----------+----------+
        |          |          |
        v          v          v
    CI systems  UIs and    Webhook/Event Sink
                operations
                tools
```

Do not require a message broker for the initial implementation.

---

## 23. Integrations

Solder is universal. It has no integration built for a specific product, and any tool, whether a CI system, a UI, an operations tool or a script, integrates through the same public surfaces:

- **CRDs:** create and change Repositories and Applications, and read Revisions, with ordinary Kubernetes API calls and RBAC;
- **status:** Application sync and health state, conditions such as `Ready`, the deployed and desired revisions, and the diagnosis; Revision phase, plan, health and failure;
- **Events:** the Kubernetes Events of the event model in section 22;
- **metrics:** Prometheus metrics for reconciles, Revisions and lifecycle events;
- **CLI:** `solder` commands, some of which print JSON for machines.

A CI system that promotes desired state commits to Git and, if it wants the result, watches the Revision for that commit until it is Healthy or Failed. It does not push deployment commands into the cluster.

Promotions should reference immutable artifacts:

```text
registry.example.com/payments@sha256:abc123
```

rather than mutable tags:

```text
payments:latest
```

---

## 24. Integration Boundaries

Solder has no mandatory UI, and nothing in Solder depends on a particular integrating product.

External tools:

- MUST use Solder's public Kubernetes API, never private controller internals;
- perform actions such as plan, approve, sync, rollback, suspend and resume through the same CRD fields and annotations the CLI uses, under their own RBAC;
- read state from status, Events and metrics rather than from logs;
- are optional: Solder behaves the same whether or not any tool is watching.

New integration needs are met by extending these public surfaces for every consumer, not by adding product-specific fields, controllers or code paths.

---

## 25. Optional AI Integration

AI belongs above deterministic diagnosis.

Solder produces structured evidence:

```json
{
  "application": "payments",
  "revision": "8c51af2",
  "failure": "DeploymentUnavailable",
  "rootResource": "Deployment/payments-api",
  "causalChain": [
    "ReplicaSet unavailable",
    "Pod CreateContainerConfigError",
    "Secret payments-db-v2 missing"
  ]
}
```

Any consumer can ask an LLM to explain:

```text
The deployment cannot start because the new revision references
Secret payments-db-v2, which does not exist. The reference was
introduced by the current desired revision.
```

AI MUST NOT be required to:

- calculate desired/live diff;
- determine field ownership;
- choose what to apply;
- decide whether health passed;
- generate rollback state;
- override safety policy.

Future AI recommendations must be presented as recommendations unless explicitly routed through deterministic policy/approval mechanisms.

---

## 26. Failure Handling

Failures should be classified.

Suggested categories:

```text
SourceFailure
AuthenticationFailure
RenderFailure
ValidationFailure
ConflictFailure
ApplyFailure
PruneFailure
HealthFailure
TimeoutFailure
RollbackFailure
InternalFailure
```

Every failure should expose:

- stable machine-readable reason;
- concise human message;
- affected resource if applicable;
- Revision;
- timestamp;
- retryability;
- causal details;
- relevant Kubernetes Events.

Avoid giant opaque error strings.

---

## 27. Suspension

Applications need a safe stop mechanism:

```yaml
spec:
  suspend: true
```

Suspension stops mutation/reconciliation while retaining visibility.

CLI:

```text
solder suspend payments
solder resume payments
```

Solder should continue enough observation to report state appropriately, but MUST NOT silently apply desired changes while suspended.

---

## 28. Multi-Tenancy and RBAC

Initial design should permit:

1. cluster-wide operator;
2. namespace-restricted deployment mode later.

Important controls:

- allowed destination namespaces;
- Repository Secret access;
- who can create/update Applications;
- who can approve manual sync;
- who can rollback;
- who can force SSA conflicts;
- who can suspend/resume;
- who can alter Repository credentials.

Manual approval is a write to a Kubernetes API object/subresource or an equivalent explicit operation, therefore Kubernetes RBAC can remain the authorization foundation.

Avoid inventing a parallel RBAC database.

---

## 29. Secrets and Diff Safety

Never show Secret values in:

- plan output;
- Revision status;
- CLI;
- logs;
- Kubernetes Events;
- metrics;
- AI context by default.

For Secret changes, report safe metadata such as:

```text
~ Secret/payments/database
  data changed: 2 keys
  values: REDACTED
```

Do not persist plaintext secret diffs.

---

## 30. Desired-State Validation

Before apply:

1. decode all manifests;
2. reject malformed resources;
3. validate basic metadata;
4. prevent duplicate resource identities in one render;
5. check destination restrictions;
6. discover API availability;
7. perform server-side dry-run where useful;
8. calculate conflicts;
9. build plan;
10. only then mutate according to policy.

Where possible, a failed resource should be detected before partial deployment begins.

Solder must still assume Kubernetes operations are not globally transactional.

---

## 31. Revision Identity

Do not assume a short Git SHA alone is globally unique.

Revision identity should include enough information to distinguish:

- Application;
- source repository;
- resolved source revision;
- render configuration/hash;
- relevant desired-state inputs.

Internally compute a deterministic desired-state fingerprint.

Example concept:

```text
desiredStateHash = SHA256(
  resolved source identity
  + render configuration
  + normalized rendered object identities/content
)
```

This permits reliable no-op detection and audit correlation.

---

## 32. Status Size and etcd Safety

Kubernetes objects have practical size limits and etcd is not an artifact store.

Rules:

- summaries in CRD status;
- bounded per-resource plan details;
- no unbounded logs;
- no full repository snapshots;
- no image/SBOM payloads;
- no giant rendered manifest archives;
- configurable revision history;
- truncate diagnostics safely with explicit `truncated: true`;
- external integrations may persist richer history later.

Solder must remain lightweight at the storage layer as well as runtime layer.

---

## 33. API Evolution

Start with:

```text
v1alpha1
```

Requirements:

- generated deepcopy/client code;
- OpenAPI schema validation;
- defaulting where appropriate;
- Conditions following Kubernetes conventions;
- explicit enum validation;
- backwards-compatible evolution whenever practical;
- conversion strategy before introducing incompatible versions.

Avoid prematurely promising API stability during alpha.

---

## 34. Testing Strategy

### Unit tests

High coverage for:

- normalization;
- diff;
- plan classification;
- graph inference;
- apply ordering;
- prune ordering;
- health evaluators;
- rollback planning;
- status transitions;
- secret redaction;
- desired-state hashing.

### Controller tests

Use Kubernetes/controller-runtime test environments where appropriate for:

- reconciliation;
- CRD status;
- finalizers;
- owner references;
- conflict handling;
- retry behavior.

### Integration tests

Run against real lightweight Kubernetes clusters such as Kind.

Scenarios:

```text
Git -> Application -> Deployment
Git update -> Plan -> Apply
manual approval
drift -> report
drift -> self-heal
failed rollout
automatic rollback
prune
SSA conflict
controller restart mid-deployment
source authentication failure
bad Helm render
CRD + CR deployment
```

### End-to-end

CI should create an ephemeral cluster, install Solder, use a fixture Git repository and prove the complete lifecycle.

### Performance

Benchmark:

- 10 Applications;
- 100 Applications;
- 1,000 Applications where feasible;
- large Applications with hundreds/thousands of resources;
- source cache behavior;
- drift-watch load;
- memory at idle and under reconciliation;
- reconcile latency.

Do not claim performance advantages without measurements.

---

## 35. Progressive Delivery — Later Phase

Do not overload v0.1, but preserve room for deployment strategies.

Future API:

```yaml
strategy:
  type: canary

  steps:
    - traffic: 10
      duration: 5m

    - traffic: 50
      duration: 10m

    - traffic: 100
```

Potential future strategies:

```text
rolling
canary
blueGreen
manualStages
```

Traffic manipulation should use adapters for actual ingress/service-mesh technologies rather than baking vendor-specific behavior into the core.

Health gates remain deterministic.

---

## 36. Multi-Cluster — Later Phase

The first implementation should perfect local-cluster reconciliation.

Future multi-cluster options may include:

- one Solder controller per cluster;
- central desired-state management with distributed agents;
- cluster registration;
- environment promotion;
- fleet status aggregation.

Avoid turning the first version into a centralized management plane.

The preferred long-term security model should retain pull-based, cluster-local reconciliation where possible.

---

## 37. OCI Desired State — Later Phase

Solder should eventually support immutable OCI artifacts as desired-state sources.

Concept:

```yaml
kind: Repository
spec:
  type: oci
  oci:
    url: oci://registry.example.com/platform/payments
    digest: sha256:...
```

This fits well with immutable artifact promotion.

Git remains the first implementation priority.

---

## 38. Notifications and External Events — Later Phase

Potential sinks:

- generic webhook;
- Slack/Teams adapters through external systems;
- CloudEvents-compatible endpoint;
- OpenTelemetry events.

Do not place notification-provider complexity in the reconciliation engine. Define a small event sink interface.

---

## 39. Git Webhooks

Polling must work without inbound connectivity.

Optional webhook receiver may later reduce latency, but should be a wake-up signal only:

```text
Webhook says repository changed
        |
        v
enqueue reconciliation
        |
        v
Solder independently fetches/verifies current source
```

Never trust webhook payload alone as desired state.

A webhook/API server must remain optional.

---

## 40. Installation Modes

### Standard

Cluster-scoped controller:

```text
solder install
```

### Helm

Provide a small official chart:

```text
charts/solder
```

### Raw manifests

Provide generated manifests for environments avoiding Helm.

### Future restricted mode

Namespace-scoped or restricted destination mode for multi-tenant clusters.

---

## 41. Finalizers and Deletion Semantics

Application deletion is dangerous and must have explicit semantics.

Deleting an Application should NOT unexpectedly destroy production workloads by default.

Recommended behavior:

- remove Solder management metadata/finalization safely;
- preserve workloads unless an explicit deletion policy requests cascading cleanup.

Potential policy:

```yaml
spec:
  deletionPolicy: Orphan
```

Future alternative:

```text
DeleteManagedResources
```

must be explicit.

Repository deletion should be blocked or clearly conditioned when Applications still reference it, or Applications should surface a broken reference without losing existing workloads.

---

## 42. Garbage Collection

Garbage collection covers:

- old Revisions;
- stale local Git cache;
- temporary rendering data;
- completed internal work;
- orphaned metadata where safe.

GC MUST never confuse “old history” with “resource eligible for Kubernetes deletion.”

---

## 43. Idempotency and Crash Recovery

The controller may crash at any point.

Every stage must tolerate restart:

```text
after Revision creation
after first resource apply
mid-prune
during health observation
during rollback
```

Never depend solely on in-memory workflow state.

Use Kubernetes object state, observed generations, Revision phase/status and actual cluster state to reconstruct progress.

Applying the same desired state repeatedly should converge without unintended side effects.

---

## 44. Leader Election and Availability

Support controller-runtime leader election.

A normal highly available installation may run multiple controller replicas while only the elected leader performs active reconciliation as appropriate.

Source cache can remain replica-local; losing it affects performance, not correctness.

---

## 45. Rate Limiting and Safety

Protect both Solder and the Kubernetes API.

Implement:

- workqueue rate limiting;
- source-fetch rate limiting;
- retry backoff;
- per-Application locking/serialization;
- bounded concurrency;
- context timeouts;
- configurable health timeout;
- protection against reconcile storms;
- jittered source polling.

A bad Application must not cause an uncontrolled tight loop.

---

## 46. Supply-Chain Capabilities

Solder is not a scanner or signing system, but should understand provenance.

Future policies may verify:

- immutable digest is used;
- image signature exists;
- expected attestation exists;
- SBOM reference exists;
- artifact originated from an allowed pipeline.

CI systems can produce these artifacts. Solder can eventually enforce declarative policy before deployment.

Keep verification adapters separate from the core planner.

---

## 47. Policy Engine — Future

A future policy stage can sit between Plan and Apply:

```text
Render
  -> Plan
  -> Policy
  -> Approval
  -> Apply
```

Potential checks:

- no `latest` images;
- resource limits required;
- privileged containers forbidden;
- production requires signed artifact;
- deletion of PVC requires explicit approval;
- certain namespaces forbidden;
- minimum replica count.

Prefer integration with established policy ecosystems or a constrained declarative system over inventing an unsafe general-purpose scripting language.

---

## 48. User Experience Principles

Solder should answer four questions quickly:

1. **What does Git want?**
2. **What is actually running?**
3. **What will Solder change?**
4. **If it failed, why?**

Bad:

```text
Application degraded.
```

Good:

```text
payments: Degraded

Desired revision: 8c51af2
Deployed revision: 8c51af2

Deployment/payments-api unavailable
  -> Pod cannot start
  -> Secret/payments-db-v2 is missing

Introduced in current desired revision.
```

CLI output should optimize for useful operator information rather than dumping Kubernetes objects.

---

## 49. Example End-to-End Workflow

Developer commits:

```text
9a71bc2 Fix payment timeout
```

The CI system:

```text
Pipeline #4821

Build             PASS
Unit Tests        PASS
Integration       PASS
Security Scan     PASS
SBOM              GENERATED
Sign              PASS
Push              PASS

Artifact:
registry.example.com/payments@sha256:abc123
```

The CI system updates environment Git:

```yaml
image:
  repository: registry.example.com/payments
  digest: sha256:abc123
```

Environment commit:

```text
21d83ab deploy(payments): promote build 4821
```

Solder:

```text
Repository observes 21d83ab
        |
        v
Application/payments reconciles
        |
        v
Revision/payments-21d83ab created
        |
        v
Plan

~ Deployment/payments-api
    image:
      sha256:old -> sha256:abc123
        |
        v
Apply
        |
        v
Observe
        |
        v
Healthy
```

Any tool reading Solder's status can then follow the lineage:

```text
Pod
 -> ReplicaSet
 -> Deployment
 -> Solder Revision 21d83ab
 -> Artifact sha256:abc123
 -> Source Commit 9a71bc2
```

---

## 50. Example Failure Workflow

A new commit references a nonexistent Secret.

```text
Solder detects new desired revision
        |
        v
Plan succeeds
        |
        v
Apply
        |
        v
Deployment does not become available
        |
        v
Health graph

Deployment unavailable
 -> ReplicaSet
 -> Pod CreateContainerConfigError
 -> Secret/payments-db-v2 missing
        |
        v
Revision = Failed
        |
        v
failurePolicy = rollback
        |
        v
Resolve previous healthy Revision
        |
        v
Generate rollback plan
        |
        v
Apply previous desired state
        |
        v
Healthy
```

Final state:

```text
Desired:  failed-new-revision
Deployed: previous-good-revision
Sync:     OutOfSync
Health:   Healthy
Revision: RolledBack
```

This distinction is essential.

---

## 51. MVP Definition

A useful MVP should include only enough to prove Solder's architectural advantage.

### MVP scope

- Go operator;
- Repository CRD;
- Application CRD;
- Revision CRD;
- Git HTTPS/SSH;
- plain YAML;
- Kustomize;
- Helm;
- source cache;
- deterministic normalization/diff;
- Change Plan;
- manual and automatic sync;
- SSA;
- ownership metadata;
- pruning;
- basic dependency graph;
- built-in Deployment/StatefulSet/DaemonSet/Job/Pod/PVC health;
- drift detection;
- self-heal;
- Revision history;
- rollback to previous healthy revision;
- Conditions/Events;
- metrics;
- CLI;
- Helm/raw installation;
- Kind-based E2E suite.

### Explicitly defer from MVP

- web UI;
- AI;
- progressive delivery;
- multi-cluster;
- OCI source;
- notification provider zoo;
- sophisticated custom health DSL;
- broad supply-chain policy engine.

---

## 52. Implementation Phases

### Phase 0 — Foundation

- initialize Go repository;
- controller-runtime project;
- API group;
- CRD generation;
- lint/test/build;
- container image;
- local Kind development;
- CI;
- security baseline.

### Phase 1 — Source + Render

- Repository controller;
- Git authentication;
- source cache;
- revision resolution;
- YAML renderer;
- Kustomize renderer;
- Helm renderer;
- normalization;
- validation.

Success criterion: Solder can reproducibly render desired state for an Application.

### Phase 2 — Plan

- live resource lookup;
- SSA-aware normalization;
- diff engine;
- plan model;
- Revision creation;
- CLI plan output;
- secret redaction.

Success criterion: Solder can accurately explain what would change without mutating the cluster.

### Phase 3 — Apply

- SSA engine;
- automatic/manual sync;
- apply ordering;
- ownership metadata;
- status transitions;
- basic prune.

Success criterion: a Git change safely converges into Kubernetes.

### Phase 4 — Health + Graph

- graph engine;
- relationship inference;
- health evaluators;
- rollout observation;
- deterministic causal diagnosis.

Success criterion: Solder explains whether deployment succeeded and where common failures originate.

### Phase 5 — Drift + Self-Heal

- managed resource watches;
- drift calculation;
- drift status;
- self-heal policy;
- conflict safety.

Success criterion: out-of-band managed changes are detected and optionally reconciled.

### Phase 6 — Revision + Rollback

- history retention;
- previous healthy state resolution;
- rollback planning;
- rollback execution;
- retry-loop protection.

Success criterion: failed deployments can deterministically return to the last healthy deployed state.

### Phase 7 — CLI + Operational Hardening

- complete CLI;
- metrics;
- Events;
- OTel hooks;
- HA/leader election;
- scale testing;
- documentation;
- Helm chart;
- upgrade tests.

### Phase 8 — Integration Surfaces

- typed lifecycle events;
- stable status, Events and metrics for any external tool;
- documented public API for observers and operators.

---

## 53. Definition of Done for v0.1

v0.1 is ready when an operator can:

1. install Solder with one simple command/chart;
2. create a Repository;
3. create an Application;
4. see the exact source revision;
5. render YAML/Kustomize/Helm;
6. preview a safe plan;
7. deploy automatically or approve manually;
8. observe rollout health;
9. see deterministic failure diagnosis;
10. detect drift;
11. optionally self-heal;
12. view bounded revision history;
13. rollback a failed release;
14. operate all core functions through `solder`;
15. run without Redis/database/UI;
16. restart the controller mid-operation without corrupting state;
17. pass E2E tests on Kind;
18. expose metrics and useful Kubernetes Events.

---

## 54. Engineering Guardrails for the AI Coder

These requirements are mandatory unless deliberately changed in the architecture.

### Do

- write idiomatic Go;
- keep interfaces small;
- use context throughout I/O and reconciliation;
- wrap errors with useful context;
- use typed errors/reasons where lifecycle logic depends on them;
- prefer Kubernetes conventions;
- keep controllers thin and domain logic testable outside controllers;
- make deterministic functions pure where practical;
- aggressively test diff/planner/graph/health logic;
- use SSA;
- redact secrets centrally;
- bound caches/status/history;
- document public APIs;
- use dependency injection where it improves tests;
- keep packages cohesive;
- make failure modes observable;
- preserve backwards compatibility once APIs mature.

### Do not

- introduce Redis/PostgreSQL “just in case”;
- split internal modules into microservices without a proven need;
- make a UI dependency;
- put LLM calls inside reconciliation;
- store credentials in CRDs;
- log Secrets;
- shell out unnecessarily;
- create one CRD per internal concept;
- store complete unlimited manifest history in etcd;
- silently force SSA conflicts;
- silently delete resources;
- make Application deletion cascade workloads by default;
- implement GitOps as CI pushing kubectl commands;
- build product-specific integrations into Solder;
- use mutable image tags as the recommended production pattern.

---

## 55. Architectural Interfaces

Useful conceptual interfaces:

```go
type Source interface {
    Resolve(ctx context.Context, ref SourceRef) (ResolvedSource, error)
    Fetch(ctx context.Context, source ResolvedSource) (Workspace, error)
}

type Renderer interface {
    Render(ctx context.Context, input RenderInput) ([]unstructured.Unstructured, error)
}

type Normalizer interface {
    Normalize(obj *unstructured.Unstructured) (*unstructured.Unstructured, error)
}

type GraphBuilder interface {
    Build(resources []Resource) (*ResourceGraph, error)
}

type Planner interface {
    Plan(ctx context.Context, desired []Resource, live []Resource) (*Plan, error)
}

type Applier interface {
    Apply(ctx context.Context, plan *Plan) (*ApplyResult, error)
}

type HealthEvaluator interface {
    Evaluate(ctx context.Context, resource Resource, graph *ResourceGraph) HealthResult
}

type RollbackPlanner interface {
    PlanRollback(ctx context.Context, current, target DesiredState) (*Plan, error)
}

type EventSink interface {
    Publish(ctx context.Context, event Event) error
}
```

These are conceptual boundaries, not a requirement to over-abstract the first implementation.

---

## 56. Data Flow

```text
Git Repository
     |
     v
Source Adapter
     |
     v
Workspace
     |
     v
Renderer
     |
     v
Desired Objects
     |
     +--> Normalizer / Validator
     |
     v
Desired State Fingerprint
     |
     +-------------------+
     |                   |
     v                   v
Resource Graph        Live Reader
     |                   |
     +---------+---------+
               |
               v
             Planner
               |
               v
          Change Plan
               |
               v
            Revision
               |
               v
          Policy/Approval
               |
               v
             Applier
               |
               v
          Kubernetes API
               |
        +------+------+
        |             |
        v             v
      Health         Drift
        |             |
        +------+------+
               |
               v
       Application Status
```

---

## 57. Competitive Product Philosophy

Solder should differentiate through architecture and operator experience rather than feature-count copying.

Its identity:

```text
Small control plane
+ Kubernetes-native state
+ first-class plans
+ dependency graph
+ deterministic diagnosis
+ first-class rollback
+ drift/self-heal
+ end-to-end provenance
+ clean CI and operations integration
```

The project should resist becoming a monolithic platform. Rich visualization and pipeline orchestration belong in other tools, which integrate through Solder's public surfaces.

Solder remains the focused, trustworthy reconciliation layer between desired state and Kubernetes.

---

## 58. Positioning

Solder's standalone positioning:

> **Solder — GitOps that sticks.**

Technical description:

> **A lightweight, deterministic, Kubernetes-native GitOps reconciliation engine built in Go.**

---

## 59. Initial README-Level Pitch

Solder is a lightweight GitOps controller for Kubernetes.

It watches desired state, renders it, shows exactly what will change, reconciles it using Server-Side Apply, understands resource dependencies, observes rollout health, detects drift, self-heals when configured, and can deterministically roll back failed deployments.

It runs as a small Kubernetes operator without requiring Redis, PostgreSQL or a mandatory UI.

Use Solder by itself through Kubernetes and its CLI, or connect any CI system, UI or operations tool through its public CRDs, status, Events and metrics.

**GitOps that sticks.**

---

## 60. Final Architectural Rule

When making future design decisions, use this test:

> **Does this feature help Solder reliably connect desired state to actual Kubernetes state?**

If yes, it may belong in Solder.

If it primarily builds or tests artifacts, it probably belongs in a CI system.

If it primarily visualizes, explains or provides broad cluster operations, it probably belongs in a separate tool built on Solder's public API.

If it can be implemented using standard Kubernetes mechanisms without adding another service, prefer the Kubernetes-native solution.

The desired end state is not the biggest GitOps platform.

It is the smallest trustworthy reconciliation engine that gives operators exceptional visibility into **what will change, what changed, whether it worked, why it failed, and how to recover**.
