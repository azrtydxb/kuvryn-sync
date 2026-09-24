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
  serviceAccountName: config-deployer
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
  serviceAccountName: payments-deployer
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
  serviceAccountName: store-deployer
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
```

A failure rolls back to the previous healthy Revision at once and holds the
failed one until a new commit arrives, so `maxAttempts` matters only when there
is nothing to roll back to. The controller records rollback transitions in
Revision status and emits lifecycle Events; see [Rollback](concepts.md#rollback).

## Diagnosis down to PersistentVolumes

The `admin` role bound in the destination namespace lets diagnosis read what
it needs there. PersistentVolumes are cluster-scoped, so following a claim to
its volume needs a ClusterRole:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: solder-diagnosis-volumes
rules:
  - apiGroups: [""]
    resources: ["persistentvolumes"]
    verbs: ["get"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: payments-deployer-volumes
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: solder-diagnosis-volumes
subjects:
  - kind: ServiceAccount
    name: payments-deployer
    namespace: default
```

```sh
solder diagnose payments -n default
solder graph payments -n default -o dot | dot -Tsvg > payments.svg
```

## Tracing with Helm

```yaml
# values.yaml
extraEnv:
  - name: OTEL_EXPORTER_OTLP_ENDPOINT
    value: http://otel-collector.observability:4317
  - name: OTEL_RESOURCE_ATTRIBUTES
    value: deployment.environment=prod
```
