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

## Upgrading from 0.2.x

The next release removes one field and adds diagnosis, tracing, and cache
pruning. Apply the new CRDs before the new image, as always. Then:

1. **Stop sending Revision `spec.provenance`.** The field is removed from this
   release's CRDs and installer. Nothing in Solder read or set it. The API
   server prunes it from stored Revisions when it reads them, and removes it
   for good on their next write; a client that still sends it is rejected
   under strict field validation. Solder has no product-specific integrations;
   integrate through the CRDs, status, Events, and the CLI.
2. **Grant the Application service accounts read access for diagnosis.**
   Applications now record why they are not Healthy in `status.diagnosis`
   and emit a `Diagnosed` Warning Event. Solder reads the objects below the
   managed resources as the Application's service account: `list` on Pods,
   ReplicaSets and EndpointSlices, and `get` on what they refer to, in the
   destination namespace. PersistentVolumes are cluster-scoped and need a
   ClusterRole and ClusterRoleBinding. Check each account with
   `kubectl auth can-i --list --as system:serviceaccount:<namespace>:<name>`.
   Without a grant, diagnosis stops higher up and reconciliation is
   unaffected. See
   [Diagnosis permissions](operations.md#diagnosis-permissions).
3. **Check Helm release names.** `spec.source.render.helm.releaseName` must
   now follow Helm's naming rule; see the `releaseName` entry in the
   [API reference](api.md#source-and-render-fields) and the
   [changelog](https://github.com/azrtydxb/solder/blob/main/CHANGELOG.md).
4. **Turn on tracing if you want it.** Tracing now exports: set
   `OTEL_EXPORTER_OTLP_ENDPOINT` (or `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT`) on
   the manager, with Helm through the new `extraEnv` value. Export is OTLP
   gRPC only, and the service name defaults to `solder`. Without the
   variables nothing changes. See
   [Metrics and tracing](operations.md#metrics-and-tracing).
5. **Size the cache volume.** Each replica now prunes its source cache every
   hour, so it holds the commits retained Revisions and Repositories refer to
   plus Helm charts used within a day, instead of growing forever. Bare
   clones still keep every Git object they have fetched, so size the `/tmp`
   volume for each Repository's history as well as your `spec.history`
   limits and number of Repositories;
   the chart's volume is an `emptyDir`, so set an `ephemeral-storage` request
   in `resources` if nodes are tight. See [Source cache](operations.md#source-cache).
6. **Build with Go 1.26.** Building Solder from source needs Go 1.26 or later,
   as `go.mod` has required since 0.2.0; the repository's devcontainer
   provides it. Release images are unaffected.

## Upgrading from 0.1.x to 0.2.0

Release 0.2.0 changes how Solder is installed and what it will do on an
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
