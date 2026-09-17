# Install Solder

Solder can be installed from raw Kubernetes manifests or from the alpha Helm
chart. Both paths install the same CRDs and controller.

## Requirements

- Kubernetes cluster.
- `kubectl` with cluster-admin permission for CRD installation.
- `helm` if using the chart.
- Network access from the controller Pod to configured Git remotes.

## Published image

Release images are published to GHCR:

```text
ghcr.io/azrtydxb/solder:<tag>
```

Use immutable release tags such as `v0.1.10` or pin digests in production.

## Raw manifests

Generate or use the checked-in installer bundle:

```sh
make build-installer IMG=ghcr.io/azrtydxb/solder:v0.1.10
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
  --set image.tag=v0.1.10
```

Verify:

```sh
kubectl -n solder-system rollout status deployment/solder-controller-manager
kubectl api-resources --api-group=solder.io
```

## Git credentials

For private Git repositories, create a Secret in the same namespace as the
Repository and reference it with `spec.git.auth.secretRef.name`.

```sh
kubectl create secret generic platform-git \
  --from-literal=username=git \
  --from-literal=password="$GITHUB_TOKEN"
```

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

## Development validation on KW

This repository does not use local Docker for image builds or Kind validation.
Use the KW cluster BuildKit/Kubernetes path:

```sh
helm template solder charts/solder >/tmp/solder-chart.yaml
kubectl apply --dry-run=server -f config/crd/bases
kubectl apply --dry-run=server -f /tmp/solder-chart.yaml -n solder-system
```

Images are built with the KW BuildKit service, not local Docker:

```sh
make kw-buildkit IMG=192.168.10.131:5000/solder:dev
```

E2E tests consume a prebuilt image by default and do not build or load a local
Docker image unless explicitly requested. The suite includes the product path:
Repository fetch from Git, Application render/apply, Revision health, and
applied workload verification. CI enables the `e2e` build tag through the lint
configuration so the gated test package is type-checked as well as run by
`make test-e2e`.

```sh
make test-e2e-existing-cluster IMG=192.168.10.131:5000/solder:dev
```

## Uninstall

```sh
helm uninstall solder -n solder-system
kubectl delete -f config/crd/bases
```

Deleting CRDs deletes Solder custom resources. Managed workload deletion depends
on each Application's `deletionPolicy` and Kubernetes owner/reference behavior.
