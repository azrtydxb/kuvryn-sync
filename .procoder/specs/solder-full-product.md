# solder-full-product

Status: complete

Source document: `solder-full-spec.md`

## Problem

Solder needs a complete implementation track for a lightweight, deterministic,
Kubernetes-native GitOps reconciliation engine. The product must connect desired
state in Git to actual state in Kubernetes without becoming a large platform,
without requiring Redis/PostgreSQL/message brokers/UI services, and without
mixing CI, visualization, or AI responsibilities into correctness-critical
reconciliation.

## Users

- Platform operators need safe GitOps reconciliation with clear plans, drift
  detection, rollback, and deterministic failure diagnosis.
- Application teams need Git changes to converge into Kubernetes safely and
  observably, with manual approval available when needed.
- Security and release owners need least-privilege credentials, redacted secret
  handling, provenance, immutable artifact references, and auditability.
- Dhole pipeline users need Solder to observe and report deployment outcomes
  after artifact promotion rather than pushing kubectl commands from CI.
- Kuvryn operators need Solder CRDs, events, resource graphs, plans, drift, and
  provenance to power visualization and operations without private internals.

## In scope

- Go controller-runtime operator and `solder` CLI.
- `Repository`, `Application`, and `Revision` CRDs under `solder.io/v1alpha1`.
- Git HTTPS/SSH desired-state sources with Kubernetes Secret references.
- Plain YAML, Kustomize, and Helm rendering normalized to Kubernetes objects.
- Source cache, source revision resolution, validation, and bounded status.
- Deterministic desired/live diff, Change Plan, and redacted plan output.
- Manual and automatic sync, Server-Side Apply, explicit conflict handling,
  ownership metadata, apply ordering, and safe pruning.
- Resource dependency graph, health evaluators, rollout observation, and
  deterministic causal diagnosis.
- Drift detection and optional self-heal for managed fields/resources.
- Revision history, desired-state fingerprints, rollback to last healthy state,
  retry-loop protection, and honest desired/deployed revision reporting.
- Conditions, Kubernetes Events, metrics, optional OpenTelemetry boundaries,
  structured logs, HA/leader election, rate limiting, garbage collection, and
  etcd-safe bounded storage.
- Installation through generated manifests, Helm chart, and CLI-assisted setup.
- Kind-based E2E tests and performance/scale measurements.
- Stable provider-neutral integration contracts for Dhole, Kuvryn, lifecycle
  events, and end-to-end source-to-Pod lineage.

## Out of scope for v0.1

- Mandatory web UI.
- Internal user/account database or parallel RBAC system.
- Embedded SSO.
- Redis, PostgreSQL, message broker, or separate repository microservice.
- CI pipeline engine, container builder, artifact registry, or Git server.
- AI that mutates desired state or participates in reconciliation correctness.
- Progressive delivery strategies beyond preserving API room.
- Multi-cluster management plane.
- OCI desired-state source implementation.
- Notification-provider zoo.
- Broad supply-chain policy engine.
- Unrestricted custom health scripting runtime.

## Constraints

- Kubernetes is the operational database; status/history must remain bounded.
- Pull-based reconciliation is the default; CI must not need broad cluster
  deployment credentials when Solder is used.
- SSA is the primary mutation mechanism and conflicts fail by default.
- Sync state and health state stay independent.
- Every deployment attempt creates an auditable Revision.
- Secret values must never appear in status, plan output, logs, Events, metrics,
  or AI/explanation context by default.
- Application deletion or pruning must not silently destroy workloads.
- A controller crash at any step must be recoverable from Kubernetes state.
- A failed desired revision must not enter an uncontrolled deploy-fail-rollback
  loop.
- Dhole, Solder, and Kuvryn remain independently useful and loosely coupled.

## Interfaces

- CRDs: `Repository`, `Application`, `Revision` in `solder.io/v1alpha1`.
- CLI: `solder version`, `install`, `status`, `repos`, `repo get`, `apps`,
  `get`, `plan`, `sync`, `history`, `revision`, `rollback`, `drift`,
  `diagnose`, `suspend`, and `resume`.
- Renderers normalize desired state to `[]unstructured.Unstructured`.
- Source, renderer, normalizer, graph, planner, applier, health, rollback, and
  event sink boundaries follow `solder-full-spec.md` section 55.
- Metrics include reconcile, source, plan, apply, drift, rollback, and
  application state families with bounded labels.
- Lifecycle events include RepositoryReady/Failed, RevisionDetected,
  PlanCreated, ApprovalRequired, DeploymentStarted/Healthy/Failed,
  RollbackStarted/Completed/Failed, DriftDetected/Reconciled, and suspension.

## Data

- Source cache is local controller storage and losing it affects performance,
  not correctness.
- Revision status stores summaries and bounded plan details, never unbounded
  manifests/logs or plaintext secret diffs.
- Desired-state identity includes source identity, render config, normalized
  rendered object identities/content, and a deterministic fingerprint.
- Old Revisions, stale source cache, temporary render data, and safe orphaned
  metadata are garbage-collected by policy.

## Edge cases

- Repository credentials are missing, malformed, revoked, or rotate during a
  reconcile.
- Git ref changes between planning and manual approval.
- Source cache is empty, corrupt, concurrently fetched, or evicted.
- Render output contains duplicate resource identities or cluster-scoped objects.
- CRDs and custom resources appear in the same desired-state change.
- SSA conflicts occur against fields owned by humans or other controllers.
- Desired/live diff contains Secret changes.
- Prune candidates include PVCs, Namespaces, CRDs, or opt-out annotations.
- Application is `Synced + Degraded`, `Drifted + Healthy`, suspended, or rolled
  back while Git still points at the failed revision.
- Controller restarts after Revision creation, mid-apply, mid-prune, during
  health observation, or during rollback.
- Health cannot infer relationships for custom resources.
- Metrics labels would become high-cardinality without guardrails.

## Failure modes

- SourceFailure, AuthenticationFailure, RenderFailure, ValidationFailure,
  ConflictFailure, ApplyFailure, PruneFailure, HealthFailure, TimeoutFailure,
  RollbackFailure, and InternalFailure are stable categories.
- Each failure exposes reason, message, affected resource when known, Revision,
  timestamp, retryability, causal details, and relevant Kubernetes Events.
- Policy or user approval gates block mutations when required.
- Rollback failure is explicit and does not hide that desired and deployed
  revisions differ.

## Acceptance criteria

- [ ] Phase 0 foundation has generated APIs, controller scaffold, CI, build,
      tests, container image path, local Kind workflow, and security baseline.
- [ ] Phase 1 can reproducibly fetch and render Git desired state for YAML,
      Kustomize, and Helm Applications.
- [ ] Phase 2 can show a deterministic, redacted plan without mutating the
      cluster.
- [ ] Phase 3 can safely converge Git changes into Kubernetes with SSA,
      ordering, metadata, manual/automatic sync, status transitions, and pruning.
- [ ] Phase 4 can build a resource graph, evaluate health, observe rollouts, and
      explain common failures with causal chains.
- [ ] Phase 5 can detect managed drift and optionally self-heal without unsafe
      conflict behavior.
- [ ] Phase 6 can retain bounded history, find the previous healthy Revision,
      plan/execute rollback, and prevent retry loops.
- [ ] Phase 7 provides complete CLI, metrics, Events, OTel hooks, HA, scale
      testing, documentation, Helm/raw install, and upgrade coverage.
- [ ] Phase 8 exposes provenance, typed lifecycle events, Dhole observation,
      Kuvryn discovery/action contracts, and source-to-Pod lineage.
- [ ] Deferred feature backlog covers progressive delivery, multi-cluster, OCI
      desired state, webhooks, notifications, policy/supply-chain, and optional AI.
- [ ] `procoder check` and `procoder test` pass before any work is called done.

## Open questions

## Decisions

- API group starts as `solder.io/v1alpha1`; real public domain must be verified
  before public release.
- Initial public API contains only `Repository`, `Application`, and `Revision`.
- Redis, PostgreSQL, broker, mandatory UI, and repo-server are not required.
- Default conflict policy is fail; force ownership is future explicit/audited
  behavior only.
- Application deletion defaults to orphan/preserve semantics.
- AI is optional and above deterministic diagnosis only.
