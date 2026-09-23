---
title: Upgrade notes
nav_order: 11
---

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

## Upgrading from 0.1.x

The next release changes how Solder is installed and what it will do on an
Application's behalf. Before upgrading:

1. **Install cert-manager.** Solder's admission webhooks (HealthCheck
   validation and approval recording) get their certificate from it.
2. **Give every Application a service account.** Set
   `spec.serviceAccountName`, or start the manager with
   `--default-service-account` (Helm `defaultServiceAccount`); Applications
   with neither are refused. Bind each account to what the Application
   deploys, for example the `admin` ClusterRole in its namespace. See the
   [security model](security.md#rbac-and-service-account-impersonation).
3. **Pin service accounts for `.solder.yaml` Applications** with the
   Repository's `spec.applicationServiceAccountName`; discovered Applications
   may no longer name one themselves.
4. **Label credential Secrets.** Git credentials need
   `solder.io/git-credentials: "true"`; registry credentials for charts and
   ImagePolicies need `solder.io/registry-credentials: "true"`; SOPS keys need
   `solder.io/decryption-key: "true"`.
5. **Check Repository URLs and SSH.** URLs must use `https`, `http`, `ssh`, or
   `git`; SSH Secrets need a `known_hosts` entry.
6. **Vendor Helm chart dependencies** for charts rendered from Git, and move
   off Kustomize remote bases.
7. **Re-approve pending manual syncs.** An `approved-revision` annotation set
   before the upgrade has no recorded approver or plan digest, so it no longer
   applies; approve again with `solder approve`.

The controller's own ClusterRole shrinks to read-only access for managed
kinds; apply the new CRDs and RBAC from the release before the new image.
The [changelog](https://github.com/azrtydxb/solder/blob/main/CHANGELOG.md)
lists every change.
