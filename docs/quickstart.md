---
title: Quickstart
nav_order: 2
---

# Quickstart

This guide deploys Kuvryn Sync and reconciles one application from Git.

## Requirements

- Kubernetes cluster with permission to install CRDs and a controller.
- `kubectl` configured for that cluster.
- [cert-manager](https://cert-manager.io), which issues the certificate for
  Kuvryn Sync's admission webhook.
- `helm` for the chart flow, or `kustomize`/`kubectl` for raw manifests.
- A Git repository containing Kubernetes manifests, Kustomize overlays, or a
  Helm chart.

## 1. Install the controller

Install cert-manager if the cluster does not have it, then the CRDs:

```sh
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/latest/download/cert-manager.yaml
kubectl -n cert-manager rollout status deployment/cert-manager-webhook
# The webhook Deployment is ready before it serves; wait until an Issuer is admitted.
until printf 'apiVersion: cert-manager.io/v1\nkind: Issuer\nmetadata: {name: probe, namespace: cert-manager}\nspec: {selfSigned: {}}\n' |
  kubectl apply --dry-run=server -f - >/dev/null 2>&1; do sleep 2; done
kubectl apply -f config/crd/bases
```

Deploy with Helm. The chart deploys the published image of its own release
(`v<appVersion>` from `charts/kuvryn-sync/Chart.yaml`), so run this from a checkout
of a release tag:

```sh
helm upgrade --install kuvryn-sync charts/kuvryn-sync \
  --namespace kuvryn-sync-system \
  --create-namespace
```

Wait for the manager. The chart names its resources `<release>-kuvryn-sync`, so
the release `kuvryn-sync` runs as `deployment/kuvryn-sync-kuvryn-sync`:

```sh
kubectl -n kuvryn-sync-system rollout status deployment/kuvryn-sync-kuvryn-sync
```

## 2. Register a Git repository

For a public repository:

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
  pollInterval: 60s
```

For a private repository, create a Secret labelled
`sync.kuvryn.io/git-credentials: "true"` and reference it from the Repository.
Kuvryn Sync refuses unlabelled Secrets.
Secret values are consumed by the controller and must not be copied into
Application specs or annotations.

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: platform-git
  namespace: default
  labels:
    sync.kuvryn.io/git-credentials: "true"
type: Opaque
stringData:
  username: git
  password: <token>
---
apiVersion: sync.kuvryn.io/v1alpha1
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

## 3. Grant Kuvryn Sync a service account to deploy with

Kuvryn Sync applies each Application as a service account, so it can only change
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

Applications you create directly set
`spec.serviceAccountName: payments-deployer`; Applications declared in
`.ksync.yaml` (next step) take it from the Repository. Alternatively, install
Kuvryn Sync with `--set defaultServiceAccount=<name>`; Kuvryn Sync then uses the
service account of that name in each Application's namespace. Applications
without a service account are refused. The `admin` role cannot create
Namespace objects, so rendered Namespaces fail as `Forbidden` unless you grant
more.

## 4. Declare Applications in Git

Add `.ksync.yaml` at the root of the repository. The Repository controller
reads this file after resolving Git and creates or updates the listed
Applications in the Repository namespace.

For monorepos, move the file into one or more subdirectories and list those
repository-relative paths on the `Repository`:

```yaml
spec:
  applicationConfigPaths:
    - teams/payments/.ksync.yaml
    - teams/search/.ksync.yaml
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
      health:
        timeout: 5m
      history:
        limit: 20
```

Discovered Applications run as the Repository's
`spec.applicationServiceAccountName`. Set it to the service account from step
3, or leave it empty to use the manager's `defaultServiceAccount`; a
`.ksync.yaml` cannot choose a service account itself.

`spec.source.repositoryRef.name` is optional in `.ksync.yaml`; when omitted,
Kuvryn Sync defaults it to the Repository that discovered the file. A full
`Application` object is also accepted when the file contains a single app. When
`spec.applicationConfigPaths` is empty, Kuvryn Sync reads the root `.ksync.yaml` by
default.

Configured `.ksync.yaml` paths must be relative to the repository, must stay
inside the repository, must be named `.ksync.yaml`, and cannot be duplicated.
Application names must be unique across all configured files. Discovered
Applications are annotated with `sync.kuvryn.io/discovered-from`, and Applications
removed from the configured files are pruned.

Renderer choices:

- `yaml`: read plain YAML documents from `path`.
- `kustomize`: run Kustomize build against `path`.
- `helm`: render a Helm chart from `path` and optional values files.

## 5. Observe reconciliation

```sh
kubectl get applications.sync.kuvryn.io,revisions.sync.kuvryn.io
kubectl describe app payments
ksync apps -n default
ksync history payments -n default
ksync plan payments -n default
```

A healthy automatic sync usually ends with:

- Application `.status.sync.state: Synced`
- Application `.status.health.state: Healthy`
- Revision `.status.phase: Healthy`

If the Application stays Progressing or turns Degraded, ask Kuvryn Sync why:

```sh
ksync diagnose payments -n default
```

It prints the chain from the unhealthy resource down to the root cause, such
as a missing Secret or an image that cannot be pulled. See
[Reading a diagnosis](troubleshooting.md#reading-a-diagnosis).

## 6. Try an update

Push a change to the Git path, then wait for polling or force a reconcile by
editing the Repository/Application metadata. Kuvryn Sync resolves the new Git commit,
creates or updates a Revision, computes a plan, applies with SSA, observes
health, and updates status.

## 7. Roll back

Roll back to the newest known-good Revision that is neither desired nor
deployed; the rollback holds until a new commit arrives:

```sh
ksync rollback payments -n default
```

Or select a Revision explicitly:

```sh
ksync rollback payments -n default --revision payments-abc123
```
