---
title: Operations
nav_order: 8
---

# Operations guide

Solder is installed from generated CRDs plus the controller manifests in
`config/` or the alpha Helm chart in `charts/solder`.

## Day-two commands

Core commands use Kubernetes CRDs directly:

- `solder apps -n <namespace>` lists Applications.
- `solder repos -n <namespace>` lists Repositories.
- `solder plan <application>` reads Revision status and prints redacted plan
  output.
- `solder history <application>` lists retained deployment attempts.
- `solder diagnose <application>` prints the latest deterministic failure.
- `solder rollback <application>` requests rollback to a healthy Revision.

Application, Repository, and Revision status remain the public integration API.
Mutation helpers update public CRDs and require exact Revision approval where
applicable.

## Manual approval

With `spec.sync.automatic: false`, Solder plans each Revision and waits. To
approve, set the Application annotation `solder.io/approved-revision` to the
Revision name, with `solder approve` or `kubectl annotate`. Anyone allowed to
update the Application can approve; use RBAC to decide who that is.

Solder's admission webhook then records, from the authenticated request:

- `solder.io/approved-by`: the Kubernetes user who approved;
- `solder.io/approved-at`: when;
- `solder.io/approved-digest`: the digest of the plan they approved
  (`status.plan.digest` on the Revision).

These annotations cannot be set or edited by hand: the webhook overwrites them
on every change. Applications discovered from `.solder.yaml` never carry
approvals from Git. Solder applies only when the approved Revision's current
plan digest still matches; if desired or live state changed after approval,
the Revision returns to AwaitingApproval with an `ApprovalStale` Event and must
be approved again. The applied Revision keeps the record in
`status.approval`, and `solder history -o json` exports it.

The webhook fails closed: while the controller is unavailable, Applications
cannot be created or updated.

## Safety defaults

- Server-Side Apply conflicts fail by default.
- Secret values are redacted from plans and CLI output.
- Sync state and health state are separate.
- Revision plan/history data is bounded.
- Rollback goes through normal reconcile machinery.

## Health checks

The manager exposes Kubernetes health/readiness probes configured by the
controller-runtime scaffold. Check rollout and logs with:

```sh
kubectl -n solder-system rollout status deployment/solder-controller-manager
kubectl -n solder-system logs deployment/solder-controller-manager -c manager
```

## Drift detection for other kinds

Solder watches a managed kind for drift only if its controller service account
may `list` and `watch` it; it checks this with a SelfSubjectAccessReview when it
first applies the kind and again every resync interval. To get immediate drift
detection for, say, cert-manager Certificates, grant the controller read access
to them:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: solder-watch-certificates
rules:
  - apiGroups: ["cert-manager.io"]
    resources: ["certificates"]
    verbs: ["list", "watch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: solder-watch-certificates
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: solder-watch-certificates
subjects:
  - kind: ServiceAccount
    name: solder-controller-manager
    namespace: solder-system
```

Without the grant, Applications managing that kind are re-checked every
`--drift-resync-interval` (default 5m); `0` disables the resync.

## High availability

Leader election is available through `--leader-elect` and enabled by the chart
values:

```yaml
replicaCount: 2
leaderElection: true
```

Use at least two replicas for controller availability, while remembering that
only the elected leader reconciles at any moment.

## Metrics and tracing

Solder registers Prometheus collectors with bounded labels for reconciliation,
plans, sync results, and health. Expose metrics using the generated service and
your cluster's monitoring stack.

An optional OpenTelemetry tracing seam exists for environments that configure a
tracer provider. Tracing must not include Secret values.

## Repository-driven Application discovery

Repositories can bootstrap Applications from `.solder.yaml` files in Git. Leave
`spec.applicationConfigPaths` empty to read the root `.solder.yaml`, or list one
or more repository-relative `.solder.yaml` paths for monorepos.

Operational rules:

- every configured path must be relative, unique, inside the repository, and
  named `.solder.yaml`;
- each file can contain one `Application` or an `applications:` list;
- Application names must be unique across all configured files;
- discovered Applications carry `solder.io/repository` and
  `solder.io/discovered-from` metadata;
- removing a discovered Application from Git prunes the managed Application CR.

## Events and Conditions

Use Kubernetes-native surfaces first:

```sh
kubectl describe app <name> -n <namespace>
kubectl get events -n <namespace> --sort-by=.lastTimestamp
kubectl get revisions.solder.io -n <namespace>
```

Solder emits lifecycle Events and writes Conditions for readiness, failure, and
rollout states.

## History retention

Set `spec.history.limit` on Applications to bound Revision count. Retention is
per Application and prevents unbounded status/API growth.

## Release validation

Release validation should use a repeatable CI or cluster environment that matches
the target architecture. Published images are built by GitHub Actions and pushed
to GHCR.
