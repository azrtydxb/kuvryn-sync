# Quickstart

This guide deploys Solder and reconciles one application from Git.

## Requirements

- Kubernetes cluster with permission to install CRDs and a controller.
- `kubectl` configured for that cluster.
- `helm` for the chart flow, or `kustomize`/`kubectl` for raw manifests.
- A Git repository containing Kubernetes manifests, Kustomize overlays, or a
  Helm chart.

## 1. Install the controller

Install the CRDs first:

```sh
kubectl apply -f config/crd/bases
```

Deploy with Helm and the published image:

```sh
helm upgrade --install solder charts/solder \
  --namespace solder-system \
  --create-namespace \
  --set image.repository=ghcr.io/azrtydxb/solder \
  --set image.tag=v0.1.10
```

Wait for the manager:

```sh
kubectl -n solder-system rollout status deployment/solder-controller-manager
```

## 2. Register a Git repository

For a public repository:

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

For a private repository, create a Secret and reference it from the Repository.
Secret values are consumed by the controller and must not be copied into
Application specs or annotations.

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: platform-git
  namespace: default
type: Opaque
stringData:
  username: git
  password: <token>
---
apiVersion: solder.io/v1alpha1
kind: Repository
metadata:
  name: platform
  namespace: default
spec:
  type: git
  git:
    url: https://github.com/example/private-platform.git
    revision: main
    auth:
      secretRef:
        name: platform-git
  pollInterval: 60s
```

Check source readiness:

```sh
kubectl get repo platform
kubectl describe repo platform
```

## 3. Create an Application

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
```

Renderer choices:

- `yaml`: read plain YAML documents from `path`.
- `kustomize`: run Kustomize build against `path`.
- `helm`: render a Helm chart from `path` and optional values files.

## 4. Observe reconciliation

```sh
kubectl get applications.solder.io,revisions.solder.io
kubectl describe app payments
solder apps -n default
solder history payments -n default
solder plan payments -n default
```

A healthy automatic sync usually ends with:

- Application `.status.sync.state: Synced`
- Application `.status.health.state: Healthy`
- Revision `.status.phase: Healthy`

## 5. Try an update

Push a change to the Git path, then wait for polling or force a reconcile by
editing the Repository/Application metadata. Solder resolves the new Git commit,
creates or updates a Revision, computes a plan, applies with SSA, observes
health, and updates status.

## 6. Roll back

Roll back to the latest healthy Revision:

```sh
solder rollback payments -n default
```

Or select a Revision explicitly:

```sh
solder rollback payments -n default --revision payments-abc123
```
