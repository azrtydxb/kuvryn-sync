---
title: Install
nav_order: 4
---

# Install Solder

Solder can be installed from raw Kubernetes manifests or from the alpha Helm
chart. Both paths install the same CRDs and controller.

## Requirements

- Kubernetes cluster.
- `kubectl` with cluster-admin permission for CRD installation.
- `helm` if using the chart.
- Network access from the controller Pod to configured Git remotes.
- [cert-manager](https://cert-manager.io). Solder's validating webhook for
  HealthChecks is served over TLS with a certificate cert-manager issues and
  injects; both the raw manifests and the Helm chart create the Issuer and
  Certificate and require cert-manager to be running first.

## Published image

Release images are published to GHCR:

```text
ghcr.io/azrtydxb/solder:<tag>
```

Use immutable release tags such as `v0.1.11` or pin digests in production.

## Raw manifests

Generate or use the checked-in installer bundle:

```sh
make build-installer IMG=ghcr.io/azrtydxb/solder:v0.1.11
kubectl apply -f dist/install.yaml
```

For local repository development you can also apply Kustomize directly:

```sh
kubectl apply -f config/crd/bases
kubectl apply -k config/default
```

## Helm chart

The alpha chart lives in `charts/solder` and expects CRDs to be installed first:

```sh
kubectl apply -f config/crd/bases
helm upgrade --install solder charts/solder \
  --namespace solder-system \
  --create-namespace \
  --set image.repository=ghcr.io/azrtydxb/solder \
  --set image.tag=v0.1.11
```

Verify:

```sh
kubectl -n solder-system rollout status deployment/solder-controller-manager
kubectl api-resources --api-group=solder.io
```

## Git credentials

For private Git repositories, create a Secret in the same namespace as the
Repository and reference it with `spec.git.auth.secretRef.name`. HTTPS remotes
use `username` and `password`, or `token`. SSH remotes need `sshPrivateKey` and
`known_hosts`; Solder rejects host keys that are not listed.

```sh
kubectl create secret generic platform-git \
  --from-literal=username=git \
  --from-literal=password="$GITHUB_TOKEN"
kubectl label secret platform-git solder.io/git-credentials=true
```

Solder only uses Secrets carrying the `solder.io/git-credentials=true` label,
so a Repository cannot send an unrelated Secret to an arbitrary Git server.

```yaml
apiVersion: solder.io/v1alpha1
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
helm template solder charts/solder >/tmp/solder-chart.yaml
kubectl apply --dry-run=server -f config/crd/bases
kubectl apply --dry-run=server -f /tmp/solder-chart.yaml -n solder-system
```

E2E tests consume a prebuilt image. Use a pullable image that matches your
cluster architecture:

```sh
make test-e2e-existing-cluster IMG=ghcr.io/azrtydxb/solder:v0.1.11
```

The E2E suite covers the product path: Repository fetch from Git, Application
render/apply, Revision health, and applied workload verification.

## Uninstall

```sh
helm uninstall solder -n solder-system
kubectl delete -f config/crd/bases
```

Deleting CRDs deletes Solder custom resources. Managed workload deletion depends
on each Application's `deletionPolicy` and Kubernetes owner/reference behavior.
