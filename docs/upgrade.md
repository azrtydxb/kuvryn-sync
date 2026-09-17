# Upgrade notes

Solder is currently `v1alpha1`. Compatibility checks are practical rather than contractual:

- CRDs are generated from Go API types with `make manifests`.
- Existing sample manifests in `config/samples` should continue to validate against generated CRDs.
- Revision history is bounded by Application policy, so upgrades must not require unbounded status data.
- Public integrations should use CRDs and Kubernetes Events, not controller internals.

Before an alpha upgrade:

```sh
make manifests generate fmt test
kubectl apply --dry-run=server -f config/crd/bases
kubectl apply --dry-run=server -f config/samples
helm template solder charts/solder >/tmp/solder-chart.yaml
kubectl apply --dry-run=server -f /tmp/solder-chart.yaml -n solder-system
```
