# Install Solder

## Raw manifests

```sh
kubectl apply -f config/crd/bases
kubectl apply -k config/default
```

## Helm chart

The alpha chart lives in `charts/solder` and expects CRDs to be installed first:

```sh
kubectl apply -f config/crd/bases
helm upgrade --install solder charts/solder --namespace solder-system --create-namespace
```

## Development validation on KW

This repository does not use local Docker for image builds or Kind validation. Use the KW cluster:

```sh
helm template solder charts/solder >/tmp/solder-chart.yaml
kubectl apply --dry-run=server -f config/crd/bases
kubectl apply --dry-run=server -f /tmp/solder-chart.yaml -n solder-system
```

Images are built with the KW BuildKit service, not local Docker:

```sh
make kw-buildkit IMG=192.168.10.131:5000/solder:dev
```

E2E tests consume a prebuilt image by default and do not build or load a local Docker image unless explicitly requested:

```sh
make test-e2e IMG=192.168.10.131:5000/solder:dev
```
