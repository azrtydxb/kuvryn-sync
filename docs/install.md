---
title: Install
nav_order: 4
---

# Install Kuvryn Sync

Kuvryn Sync can be installed from raw Kubernetes manifests or from the alpha Helm
chart. Both paths install the same CRDs and controller.

## Requirements

- Kubernetes cluster.
- `kubectl` with cluster-admin permission for CRD installation.
- `helm` if using the chart.
- Network access from the controller Pod to configured Git remotes, and to
  chart repositories, registries, notification sinks, and an OTLP collector
  if you use them.
- Go 1.26 or later only to build Kuvryn Sync from source.
- [cert-manager](https://cert-manager.io). Kuvryn Sync serves two admission
  webhooks over TLS with a certificate cert-manager issues and injects: a
  validating webhook for HealthChecks and a mutating webhook that records who
  approved an Application's Revision. Both use `failurePolicy: Fail`, so while
  the webhook is unavailable, creating or updating HealthChecks and
  Applications is refused. Both the raw manifests and the Helm chart create the
  Issuer and Certificate and require cert-manager to be running first.

## Published image

Release images are published to GHCR:

```text
ghcr.io/azrtydxb/kuvryn-sync:<tag>
```

Use immutable release tags or pin digests in production.

The manifests and the chart must be used with the image of the same release.
Releases add manager flags and webhooks that older images do not have, so a
chart or `config/` from one release with another release's image fails to
start or rejects Application writes. Install from a checkout of the release
tag, and take the image tag from it: the chart's `appVersion`, prefixed with
`v`.

The package may be private. To pull a private image, create a
`kubernetes.io/dockerconfigjson` Secret for `ghcr.io` in the install
namespace, with a token that has `read:packages`:

```bash
kubectl -n kuvryn-sync-system create secret docker-registry ghcr-pull \
  --docker-server=ghcr.io --docker-username=<user> --docker-password=<token>
```

With Helm, set `image.pullSecrets={ghcr-pull}`; the manager and console
Deployments both use it. With the raw manifests, patch the manager Deployment:

```bash
kubectl -n kuvryn-sync-system patch deployment kuvryn-sync-controller-manager \
  --type merge -p '{"spec":{"template":{"spec":{"imagePullSecrets":[{"name":"ghcr-pull"}]}}}}'
```

## Raw manifests

Generate or use the checked-in installer bundle:

```sh
make build-installer IMG=ghcr.io/azrtydxb/kuvryn-sync:v$(awk '/^appVersion:/ {print $2}' charts/kuvryn-sync/Chart.yaml)
kubectl apply -f dist/install.yaml
```

For local repository development you can also apply Kustomize directly:

```sh
kubectl apply -f config/crd/bases
kubectl apply -k config/default
```

## Helm chart

The alpha chart lives in `charts/kuvryn-sync` and expects CRDs to be installed
first. It deploys `ghcr.io/azrtydxb/kuvryn-sync:v<appVersion>` by default; set
`image.tag` only to an image built from the same commit as the chart.

```sh
kubectl apply -f config/crd/bases
helm upgrade --install kuvryn-sync charts/kuvryn-sync \
  --namespace kuvryn-sync-system \
  --create-namespace
```

Verify. The chart names its Deployment, ServiceAccount and Services
`<release>-kuvryn-sync`, so the release `kuvryn-sync` runs as `deployment/kuvryn-sync-kuvryn-sync`.
The raw manifests name it `kuvryn-sync-controller-manager` instead.

```sh
kubectl -n kuvryn-sync-system rollout status deployment/kuvryn-sync-kuvryn-sync
kubectl api-resources --api-group=sync.kuvryn.io
```

### Chart values

| Value                     | Default                               | Meaning                                                                                                                                           |
| ------------------------- | ------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------- |
| `image.repository`        | `ghcr.io/azrtydxb/kuvryn-sync`        | Manager image.                                                                                                                                    |
| `image.tag`               | `v<appVersion>`                       | Override only with an image built from the same commit as the chart.                                                                              |
| `image.pullPolicy`        | `IfNotPresent`                        | Image pull policy.                                                                                                                                |
| `image.pullSecrets`       | `[]`                                  | Names of `dockerconfigjson` Secrets in the release namespace, used by the manager and console to pull a private image.                            |
| `replicaCount`            | `2`                                   | Manager replicas; only the leader reconciles.                                                                                                     |
| `leaderElection`          | `true`                                | Passes `--leader-elect`.                                                                                                                          |
| `defaultServiceAccount`   | `""`                                  | Service account used by Applications that set none; empty refuses them. See [Security model](security.md#rbac-and-service-account-impersonation). |
| `driftResyncInterval`     | `5m`                                  | How often Applications with unwatched kinds are re-checked for drift; `0` disables it.                                                            |
| `webhookReceiver.enabled` | `false`                               | Serves push webhooks on the Service `<release>-kuvryn-sync-receiver`. See [Push webhooks](operations.md#push-webhooks).                           |
| `extraEnv`                | `[]`                                  | Extra manager environment variables, such as the `OTEL_*` tracing settings. See [Metrics and tracing](operations.md#metrics-and-tracing).         |
| `resources`               | 50m/128Mi requests, 500m/512Mi limits | Manager container resources. Add `ephemeral-storage` to account for the source cache; see [Source cache](operations.md#source-cache).             |

## Git credentials

For private Git repositories, create a Secret in the same namespace as the
Repository and reference it with `spec.git.auth.secretRef.name`. HTTPS remotes
use `username` and `password`, or `token`. SSH remotes need `sshPrivateKey` and
`known_hosts`; Kuvryn Sync rejects host keys that are not listed.

```sh
kubectl create secret generic platform-git \
  --from-literal=username=git \
  --from-literal=password="$GITHUB_TOKEN"
kubectl label secret platform-git sync.kuvryn.io/git-credentials=true
```

Kuvryn Sync only uses Secrets carrying the `sync.kuvryn.io/git-credentials=true` label,
so a Repository cannot send an unrelated Secret to an arbitrary Git server.

```yaml
apiVersion: sync.kuvryn.io/v1alpha1
kind: Repository
metadata:
  name: platform
spec:
  type: git
  git:
    url: https://github.com/example/private-platform.git
    revision: main
    auth:
      secretRef:
        name: platform-git
```

## Validation

Validate generated manifests before deploying them to a cluster:

```sh
helm template kuvryn-sync charts/kuvryn-sync >/tmp/kuvryn-sync-chart.yaml
kubectl apply --dry-run=server -f config/crd/bases
kubectl apply --dry-run=server -f /tmp/kuvryn-sync-chart.yaml -n kuvryn-sync-system
```

E2E tests deploy this checkout's manifests with a prebuilt image, so the image
must be built from the same commit. Use a pullable image that matches your
cluster architecture:

```sh
make docker-build docker-push IMG=<registry>/kuvryn-sync:<tag>
make test-e2e-existing-cluster IMG=<registry>/kuvryn-sync:<tag>
```

The E2E suite covers the product path: Repository fetch from Git, Application
render/apply, Revision health, and applied workload verification.

## Uninstall

```sh
helm uninstall kuvryn-sync -n kuvryn-sync-system
kubectl delete -f config/crd/bases
```

Deleting CRDs deletes Kuvryn Sync custom resources. Managed workload deletion depends
on each Application's `deletionPolicy` and Kubernetes owner/reference behavior.
