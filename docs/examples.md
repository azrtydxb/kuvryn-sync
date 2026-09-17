---
title: Examples
nav_order: 7
---

# Examples

## Repository-root `.solder.yaml`

```yaml
applications:
  - metadata:
      name: payments
    spec:
      source:
        path: apps/payments/overlays/prod
        render:
          type: kustomize
      destination:
        namespace: payments
      sync:
        automatic: true
        prune: true
        selfHeal: true
        conflictPolicy: fail
  - metadata:
      name: search
    spec:
      source:
        path: apps/search
        render:
          type: yaml
      destination:
        namespace: search
      sync:
        automatic: true
        conflictPolicy: fail
```

The Repository controller defaults `spec.source.repositoryRef.name` to the
Repository that discovered the file. Application names must be unique across all
configured `.solder.yaml` files.

## Monorepo `.solder.yaml` files

```yaml
apiVersion: solder.io/v1alpha1
kind: Repository
metadata:
  name: platform
spec:
  type: git
  git:
    url: https://github.com/example/platform.git
    revision: main
  applicationConfigPaths:
    - teams/payments/.solder.yaml
    - teams/search/.solder.yaml
```

Each listed file must be named `.solder.yaml`, stay inside the repository, and
can contain one or more Applications for that part of the repository. Solder
annotates discovered Applications with `solder.io/discovered-from` and prunes
previously discovered Applications removed from these files.

## Plain YAML application

```yaml
apiVersion: solder.io/v1alpha1
kind: Application
metadata:
  name: config
spec:
  source:
    repositoryRef:
      name: platform
    path: manifests/config
    render:
      type: yaml
  destination:
    namespace: default
  sync:
    automatic: true
    conflictPolicy: fail
```

## Kustomize application

```yaml
apiVersion: solder.io/v1alpha1
kind: Application
metadata:
  name: payments
spec:
  source:
    repositoryRef:
      name: platform
    path: apps/payments/overlays/prod
    render:
      type: kustomize
  destination:
    namespace: payments
  sync:
    automatic: true
    prune: true
    selfHeal: true
    conflictPolicy: fail
```

## Helm application

```yaml
apiVersion: solder.io/v1alpha1
kind: Application
metadata:
  name: store
spec:
  source:
    repositoryRef:
      name: platform
    path: charts/store
    render:
      type: helm
      helm:
        releaseName: store
        valuesFiles:
          - values.yaml
          - values-prod.yaml
  destination:
    namespace: store
  sync:
    automatic: true
    prune: true
    conflictPolicy: fail
```

## Manual approval

Disable automatic sync, inspect the plan, then approve exactly what you reviewed:

```yaml
spec:
  sync:
    automatic: false
    conflictPolicy: fail
```

```sh
solder plan payments -n default
solder sync payments -n default --revision payments-abc123
```

## Rollback on failure

```yaml
spec:
  strategy:
    type: rolling
    failurePolicy:
      action: rollback
      timeout: 5m
      maxAttempts: 2
```

The controller records rollback transitions in Revision status and emits
lifecycle Events.
