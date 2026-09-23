---
title: Quickstart
nav_order: 2
---

# Quickstart

This guide deploys Solder and reconciles one application from Git.

## Requirements

- Kubernetes cluster with permission to install CRDs and a controller.
- `kubectl` configured for that cluster.
- [cert-manager](https://cert-manager.io), which issues the certificate for
  Solder's admission webhook.
- `helm` for the chart flow, or `kustomize`/`kubectl` for raw manifests.
- A Git repository containing Kubernetes manifests, Kustomize overlays, or a
  Helm chart.

## 1. Install the controller

Install cert-manager if the cluster does not have it, then the CRDs:

```sh
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/latest/download/cert-manager.yaml
kubectl -n cert-manager rollout status deployment/cert-manager-webhook
kubectl apply -f config/crd/bases
```

Deploy with Helm and the published image:

```sh
helm upgrade --install solder charts/solder \
  --namespace solder-system \
  --create-namespace \
  --set image.repository=ghcr.io/azrtydxb/solder \
  --set image.tag=v0.1.11
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

For a private repository, create a Secret labelled
`solder.io/git-credentials: "true"` and reference it from the Repository.
Solder refuses unlabelled Secrets.
Secret values are consumed by the controller and must not be copied into
Application specs or annotations.

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: platform-git
  namespace: default
  labels:
    solder.io/git-credentials: "true"
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

## 3. Grant Solder a service account to deploy with

Solder applies each Application as a service account, so it can only change
what that account is allowed to change. Create the destination namespace, then
a service account bound to the `admin` role there:

```sh
kubectl create namespace payments
```

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: payments-deployer
  namespace: default
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: payments-deployer
  namespace: payments
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: admin
subjects:
  - kind: ServiceAccount
    name: payments-deployer
    namespace: default
```

Applications you create directly set `spec.serviceAccountName:
payments-deployer`; Applications declared in `.solder.yaml` (next step) take it
from the Repository. Alternatively,
install Solder with `--set defaultServiceAccount=<name>`; Solder then uses the
service account of that name in each Application's namespace. Applications
without a service account are refused. The `admin` role cannot create
Namespace objects, so rendered Namespaces fail as `Forbidden` unless you grant
more.

## 4. Declare Applications in Git

Add `.solder.yaml` at the root of the repository. The Repository controller
reads this file after resolving Git and creates or updates the listed
Applications in the Repository namespace.

For monorepos, move the file into one or more subdirectories and list those
repository-relative paths on the `Repository`:

```yaml
spec:
  applicationConfigPaths:
    - teams/payments/.solder.yaml
    - teams/search/.solder.yaml
```

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
          maxAttempts: 2
      health:
        timeout: 5m
      history:
        limit: 20
```

Discovered Applications run as the Repository's
`spec.applicationServiceAccountName`. Set it to the service account from step
3, or leave it empty to use the manager's `defaultServiceAccount`; a
`.solder.yaml` cannot choose a service account itself.

`spec.source.repositoryRef.name` is optional in `.solder.yaml`; when omitted,
Solder defaults it to the Repository that discovered the file. A full
`Application` object is also accepted when the file contains a single app. When
`spec.applicationConfigPaths` is empty, Solder reads the root `.solder.yaml` by
default.

Configured `.solder.yaml` paths must be relative to the repository, must stay
inside the repository, must be named `.solder.yaml`, and cannot be duplicated.
Application names must be unique across all configured files. Discovered
Applications are annotated with `solder.io/discovered-from`, and Applications
removed from the configured files are pruned.

Renderer choices:

- `yaml`: read plain YAML documents from `path`.
- `kustomize`: run Kustomize build against `path`.
- `helm`: render a Helm chart from `path` and optional values files.

## 5. Observe reconciliation

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

## 6. Try an update

Push a change to the Git path, then wait for polling or force a reconcile by
editing the Repository/Application metadata. Solder resolves the new Git commit,
creates or updates a Revision, computes a plan, applies with SSA, observes
health, and updates status.

## 7. Roll back

Roll back to the latest healthy Revision:

```sh
solder rollback payments -n default
```

Or select a Revision explicitly:

```sh
solder rollback payments -n default --revision payments-abc123
```
