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
