# Solder

[![Tests](https://github.com/azrtydxb/solder/actions/workflows/test.yml/badge.svg)](https://github.com/azrtydxb/solder/actions/workflows/test.yml)
[![Lint](https://github.com/azrtydxb/solder/actions/workflows/lint.yml/badge.svg)](https://github.com/azrtydxb/solder/actions/workflows/lint.yml)
[![E2E](https://github.com/azrtydxb/solder/actions/workflows/test-e2e.yml/badge.svg)](https://github.com/azrtydxb/solder/actions/workflows/test-e2e.yml)
[![Image](https://github.com/azrtydxb/solder/actions/workflows/image.yml/badge.svg)](https://github.com/azrtydxb/solder/actions/workflows/image.yml)

**Solder — GitOps that sticks.**

Solder is a lightweight, deterministic, Kubernetes-native GitOps controller for
applying desired state from Git to Kubernetes. It is intentionally an operator,
not a platform bundle: no Redis, PostgreSQL, broker, or mandatory UI.

Solder focuses on the product path that matters for day-two operations:

- `Repository` CRDs resolve Git branches, tags, or commits with Secret-backed auth.
- `Application` CRDs render manifests, Kustomize, or Helm charts from Git.
- Server-Side Apply is used for mutations; ownership conflicts fail by default.
- Sync state and health state are tracked separately.
- `Revision` CRDs keep bounded, redacted, auditable plan and rollout history.
- Drift detection, self-heal, pruning, rollback, retry protection, Events,
  Prometheus metrics, and optional tracing are built into the controller path.

> Status: alpha (`solder.io/v1alpha1`). The MVP is functional and covered by
> controller, CLI, contract, and product-path e2e tests, but the API may still
> change before a stable release.

## Documentation

The full documentation site is published with GitHub Pages:

**https://azrtydxb.github.io/solder/**

Start with:

- [Quickstart](docs/quickstart.md)
- [Concepts](docs/concepts.md)
- [API reference](docs/api.md)
- [CLI reference](docs/cli.md)
- [Operations](docs/operations.md)
- [Security model](docs/security.md)
- [Troubleshooting](docs/troubleshooting.md)
- [Roadmap](docs/roadmap.md)

## Quickstart

Install CRDs and deploy the controller with Helm:

```sh
kubectl apply -f config/crd/bases
helm upgrade --install solder charts/solder \
  --namespace solder-system \
  --create-namespace \
  --set image.repository=ghcr.io/azrtydxb/solder \
  --set image.tag=v0.1.10
```

Create a Git source:

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
  pollInterval: 60s
```

Create an application from a path in that repo:

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
```

Then inspect state:

```sh
kubectl get repositories.solder.io,applications.solder.io,revisions.solder.io
solder apps -n default
solder plan payments -n default
```

## Project layout

```text
cmd/                    controller manager entry point and CLI dispatch
api/v1alpha1/           public Kubernetes API types
internal/controller/    controller-runtime reconcilers
internal/               source, renderer, plan, apply, health, drift, ops packages
config/                 CRDs, RBAC, manager manifests, samples
docs/                   GitHub Pages documentation
charts/solder/          alpha Helm chart
test/e2e/               product-path Kubernetes e2e tests
solder-full-spec.md     product and engineering specification
```

## Development

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
