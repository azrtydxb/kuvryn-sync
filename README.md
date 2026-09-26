# Kuvryn Sync

[![Tests](https://github.com/azrtydxb/kuvryn-sync/actions/workflows/test.yml/badge.svg)](https://github.com/azrtydxb/kuvryn-sync/actions/workflows/test.yml)
[![Lint](https://github.com/azrtydxb/kuvryn-sync/actions/workflows/lint.yml/badge.svg)](https://github.com/azrtydxb/kuvryn-sync/actions/workflows/lint.yml)
[![E2E](https://github.com/azrtydxb/kuvryn-sync/actions/workflows/test-e2e.yml/badge.svg)](https://github.com/azrtydxb/kuvryn-sync/actions/workflows/test-e2e.yml)
[![Image](https://github.com/azrtydxb/kuvryn-sync/actions/workflows/image.yml/badge.svg)](https://github.com/azrtydxb/kuvryn-sync/actions/workflows/image.yml)

**Kuvryn Sync — an Azrty product.**

Kuvryn Sync is a lightweight, deterministic, Kubernetes-native GitOps controller for
applying desired state from Git to Kubernetes. It is intentionally an operator,
not a platform bundle: no Redis, PostgreSQL, broker, or mandatory UI.

Kuvryn Sync focuses on the product path that matters for day-two operations:

- `Repository` CRDs resolve Git branches, tags, or commits with Secret-backed auth,
  and can fetch immediately on signed GitHub or GitLab push webhooks.
- `Application` CRDs render manifests, Kustomize, or Helm charts from Git or
  from Helm/OCI chart repositories, in process, with SOPS decryption.
- Each Application applies as its own service account, so Kubernetes RBAC
  decides what it may change.
- Manual approvals are recorded against the authenticated approver and bound
  to the exact plan they approved; lifecycle notifications go to webhook or
  Slack sinks.
- Health, pruning, and drift work for any kind, with CEL `HealthCheck` rules
  for kinds kstatus cannot judge.
- Rollouts can be ordered with `dependsOn`, sync waves, and pre/post-sync hooks.
- `ImagePolicy` scans registries and commits new image digests back to Git.
- Server-Side Apply is used for mutations; ownership conflicts fail by default,
  and `adopt` takes fields over deliberately when migrating from Flux or Argo CD.
- `Revision` CRDs keep bounded, redacted, auditable plan and rollout history.
- Unhealthy Applications explain themselves: Kuvryn Sync walks the live resource
  graph down to the root cause, such as a missing Secret or an image pull
  failure, records it in `status.diagnosis`, and `ksync diagnose` and
  `ksync graph` print it.
- Prometheus metrics and, when an OTLP endpoint is configured,
  OpenTelemetry traces of every Application reconcile.
- An optional, read-only web console (`ksync console`) signs people in with
  a Kubernetes token or, optionally, OIDC such as Dex, and shows
  Applications, their diagnosis, plans, history and resources through each
  person's own Kubernetes RBAC.

![The Kuvryn Sync console Applications page](docs/images/console-applications.png)

> Status: alpha (`sync.kuvryn.io/v1alpha1`). The MVP is functional and covered by
> controller, CLI, and product-path e2e tests, but the API may still
> change before a stable release.

## Documentation

The full documentation site is published with GitHub Pages:

**https://azrtydxb.github.io/kuvryn-sync/**

Start with:

- [Quickstart](docs/quickstart.md)
- [Concepts](docs/concepts.md)
- [API reference](docs/api.md)
- [CLI reference](docs/cli.md)
- [Operations](docs/operations.md)
- [Web console](docs/console.md)
- [Security model](docs/security.md)
- [Troubleshooting](docs/troubleshooting.md)
- [Upgrade notes](docs/upgrade.md)
- [Roadmap](docs/roadmap.md)

## Quickstart

Kuvryn Sync's admission webhooks get their certificate from
[cert-manager](https://cert-manager.io), so install that first, then the CRDs
and the controller. Run this from a checkout of a release tag: the chart
deploys the image of its own release (`v<appVersion>`), and a chart from one
release does not work with another release's image.

```sh
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/latest/download/cert-manager.yaml
kubectl -n cert-manager rollout status deployment/cert-manager-webhook
# The webhook Deployment is ready before it serves; wait until an Issuer is admitted.
until printf 'apiVersion: cert-manager.io/v1\nkind: Issuer\nmetadata: {name: probe, namespace: cert-manager}\nspec: {selfSigned: {}}\n' |
  kubectl apply --dry-run=server -f - >/dev/null 2>&1; do sleep 2; done
kubectl apply -f config/crd/bases
helm upgrade --install kuvryn-sync charts/kuvryn-sync \
  --namespace kuvryn-sync-system \
  --create-namespace
kubectl -n kuvryn-sync-system rollout status deployment/kuvryn-sync-kuvryn-sync
```

Create a Git source:

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
  applicationServiceAccountName: payments-deployer
  applicationPolicy:
    allowAutomatic: true
    allowPrune: true
  pollInterval: 60s
```

Add `.ksync.yaml` at the root of that Git repository to declare Applications. For monorepos, set `spec.applicationConfigPaths` on the Repository to point at one or more nested `.ksync.yaml` files instead. Each configured path must be repository-relative, stay inside the repository, and be named `.ksync.yaml`.

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
```

Discovered Applications run as the Repository's `spec.applicationServiceAccountName` (here `payments-deployer`); a `.ksync.yaml` cannot choose a different service account — grant that account what the Applications deploy, as described in the [security model](docs/security.md). A `.ksync.yaml` also cannot switch on automatic sync, pruning, `conflictPolicy: adopt` or `deletionPolicy: DeleteManagedResources` unless the Repository's `spec.applicationPolicy` allows it; here it allows automatic sync and pruning. When the `Repository` reconciles, Kuvryn Sync discovers the configured files, defaults each Application to that Repository, and creates or updates the Application CRs. Application names must be unique across all discovered files; removed discovered Applications are pruned.

Then inspect state:

```sh
kubectl get repositories.sync.kuvryn.io,applications.sync.kuvryn.io,revisions.sync.kuvryn.io
ksync apps -n default
ksync plan payments -n default
ksync diagnose payments -n default
```

## Project layout

```text
cmd/                    controller manager entry point and CLI dispatch
api/v1alpha1/           public Kubernetes API types
internal/controller/    controller-runtime reconcilers
internal/               source, renderer, plan, apply, health, drift, graph,
                        diagnosis, ops packages
config/                 CRDs, RBAC, manager manifests, samples
docs/                   GitHub Pages documentation
charts/kuvryn-sync/          alpha Helm chart
test/e2e/               product-path Kubernetes e2e tests
kuvryn-sync-full-spec.md   product and engineering specification
```

## Development

Building from source needs Go 1.26 or later. The repository's devcontainer
provides it; see [CONTRIBUTING.md](CONTRIBUTING.md).

Generate CRDs and deepcopy code:

```sh
make manifests generate
```

Run the normal test suite:

```sh
make test
```

Run Procoder gates:

```sh
procoder test
procoder check
```

Build the controller manager binary:

```sh
make build
```

Build and publish release images through GitHub Actions. For local image
experiments, use any registry and build system appropriate for your cluster.

## License

Licensed under the Apache License, Version 2.0. See [LICENSE](LICENSE).
