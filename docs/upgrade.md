---
title: Upgrade notes
nav_order: 11
---

# Upgrade notes

Kuvryn Sync is currently `v1alpha1`. Compatibility checks are practical rather than contractual:

- CRDs are generated from Go API types with `make manifests`.
- Existing sample manifests in `config/samples` should continue to validate against generated CRDs.
- Revision history is bounded by Application policy, so upgrades must not require unbounded status data.
- Public integrations should use CRDs and Kubernetes Events, not controller internals.

Before an alpha upgrade:

```sh
make manifests generate fmt test
kubectl apply --dry-run=server -f config/crd/bases
kubectl apply --dry-run=server -f config/samples
helm template kuvryn-sync charts/kuvryn-sync >/tmp/kuvryn-sync-chart.yaml
kubectl apply --dry-run=server -f /tmp/kuvryn-sync-chart.yaml -n kuvryn-sync-system
```

## Moving from Solder 0.3.x to Kuvryn Sync 0.4.0

Kuvryn Sync is the new name of Solder, and 0.4.0 is the first release under
it. The rename is a clean break: **no migration is provided**, and nothing in
Kuvryn Sync reads a Solder object, label, annotation, or discovery file. A
Solder install keeps running untouched until you remove it.

| Solder 0.3.x                                                | Kuvryn Sync 0.4.0                                   |
| ----------------------------------------------------------- | --------------------------------------------------- |
| API group `solder.io`, e.g. `solder.io/v1alpha1`            | `sync.kuvryn.io`, e.g. `sync.kuvryn.io/v1alpha1`    |
| Labels and annotations `solder.io/<key>`                    | `sync.kuvryn.io/<key>`, with the same suffixes      |
| CLI `solder <command>`                                      | `ksync <command>`, with the same commands and flags |
| Image `ghcr.io/azrtydxb/solder`                             | `ghcr.io/azrtydxb/kuvryn-sync`                      |
| Helm chart `charts/solder`                                  | `charts/kuvryn-sync`                                |
| Namespace `solder-system`                                   | `kuvryn-sync-system`                                |
| Server-Side Apply field manager `solder`                    | `kuvryn-sync`                                       |
| Metrics `solder_*`                                          | `kuvryn_sync_*`                                     |
| Discovery file `.solder.yaml`                               | `.ksync.yaml`                                       |
| Notification headers `X-Solder-Signature`, `X-Solder-Event` | `X-Kuvryn-Sync-Signature`, `X-Kuvryn-Sync-Event`    |

To move a cluster over:

1. **Install Kuvryn Sync next to Solder.** Apply the new CRDs, then install
   the chart as `kuvryn-sync`; see [Install](install.md). The two products use
   different API groups, namespaces, field managers and labels, so each
   ignores the other's objects.
2. **Re-create your objects under `sync.kuvryn.io`.** Change `apiVersion` to
   `sync.kuvryn.io/v1alpha1` and every `solder.io/` label and annotation key to
   `sync.kuvryn.io/` in your Repository, Application, HealthCheck,
   NotificationSink and ImagePolicy manifests, and rename `.solder.yaml`
   discovery files to `.ksync.yaml` (`spec.applicationConfigPaths` must now
   name `.ksync.yaml` files). Relabel credential Secrets with
   `sync.kuvryn.io/git-credentials`, `sync.kuvryn.io/registry-credentials` and
   `sync.kuvryn.io/decryption-key`.
3. **Take over the workloads with `conflictPolicy: adopt`.** Workloads Solder
   applied keep their `solder.io/*` labels, and their fields stay owned by the
   field manager `solder`. With the default `conflictPolicy: fail`, the new
   Application's plan reports those fields as conflicts; set
   `spec.sync.conflictPolicy: adopt` to take ownership, as in
   [Migrate from Argo CD](migrate-argocd.md) and
   [Migrate from Flux](migrate-flux.md). Do not point a Solder Application and
   a Kuvryn Sync Application at the same workloads at the same time.
4. **Update what reads the old names:** dashboards and alerts on `solder_*`
   metrics, notification receivers that verify `X-Solder-Signature`, scripts
   that call the `solder` CLI, and anything that selects on `solder.io/` labels.
5. **Remove Solder** once every Application is Synced and Healthy under Kuvryn
   Sync: delete the Solder Applications with `deletionPolicy: Orphan` (the
   default), which leaves their workloads in place, then `helm uninstall` the
   Solder release and delete the `solder.io` CRDs.

Upgrade notes for Solder releases up to 0.3.0 are in the
[changelog](https://github.com/azrtydxb/kuvryn-sync/blob/main/CHANGELOG.md) and in
the [0.3.0 upgrade notes](https://github.com/azrtydxb/kuvryn-sync/blob/v0.3.0/docs/upgrade.md).
