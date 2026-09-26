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

## Upgrading to 0.7.0

Repositories now limit what Applications discovered from `.ksync.yaml` may
switch on, through the new `spec.applicationPolicy`. Everything defaults to
off, so a discovered Application that sets `spec.sync.automatic`,
`spec.sync.prune`, `spec.sync.conflictPolicy: adopt` or
`spec.deletionPolicy: DeleteManagedResources` is refused after the upgrade.
Its Repository turns `Failed` with a message naming the allowance, and
discovery creates, changes and deletes no Application of that Repository
until the file or the policy is fixed. The Applications in the cluster keep
the spec they had: one that is already automatic still syncs new commits
without approval. Allow what you want Git to control, and set the others to
manual yourself. Applications you create yourself are not affected.

Before upgrading, find the discovered Applications and what they use:

```sh
kubectl get applications.sync.kuvryn.io -A \
  -l sync.kuvryn.io/repository \
  -o 'custom-columns=NS:.metadata.namespace,NAME:.metadata.name,REPO:.metadata.labels.sync\.kuvryn\.io/repository,AUTO:.spec.sync.automatic,PRUNE:.spec.sync.prune,CONFLICT:.spec.sync.conflictPolicy,DELETE:.spec.deletionPolicy'
```

Then allow what each Repository should allow once the new CRDs are applied,
for example:

```sh
kubectl patch repositories.sync.kuvryn.io platform -n default --type merge \
  -p '{"spec":{"applicationPolicy":{"allowAutomatic":true,"allowPrune":true}}}'
```

or, to put a person back in the loop, remove `automatic: true` from the
Application in `.ksync.yaml`; discovery then sets it to manual. Until that
commit lands, patch the running one to manual:

```sh
kubectl patch applications.sync.kuvryn.io payments -n default --type merge \
  -p '{"spec":{"sync":{"automatic":false}}}'
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
| Default Helm release name `solder`                          | `kuvryn-sync`                                       |

To move a cluster over, one Application at a time:

1. **Suspend the Solder Application** so it stops applying and pruning:
   `solder suspend <app> -n <namespace>` (or set `spec.suspend: true`). Its
   workloads keep running.
2. **Install Kuvryn Sync next to Solder.** Apply the new CRDs, then install
   the chart as `kuvryn-sync`; see [Install](install.md). The two products use
   different API groups, namespaces, field managers and labels, so each
   ignores the other's objects.
3. **Re-create the objects under `sync.kuvryn.io`.** Change `apiVersion` to
   `sync.kuvryn.io/v1alpha1` and every `solder.io/` label and annotation key to
   `sync.kuvryn.io/` in your Repository, Application, HealthCheck,
   NotificationSink and ImagePolicy manifests, and rename `.solder.yaml`
   discovery files to `.ksync.yaml` (`spec.applicationConfigPaths` must now
   name `.ksync.yaml` files). Relabel credential Secrets with
   `sync.kuvryn.io/git-credentials`, `sync.kuvryn.io/registry-credentials` and
   `sync.kuvryn.io/decryption-key`. On Helm Applications that left
   `spec.source.render.helm.releaseName` empty, set it to `solder`: the default
   release name is now `kuvryn-sync`, and a different name renders different
   object names, so the takeover would create duplicates instead.
4. **Take over the workloads with `conflictPolicy: adopt`.** Workloads Solder
   applied keep their `solder.io/*` labels, and their fields are owned by the
   field manager `solder`. With the default `conflictPolicy: fail`, the new
   Application's plan reports those fields as conflicts; set
   `spec.sync.conflictPolicy: adopt` to take ownership, as in
   [Migrate from Flux](migrate-flux.md). Only the suspended Solder Application
   may still point at these workloads; never run both against them.
5. **Delete the Solder Application, orphaning its workloads.** Set
   `deletionPolicy: Orphan` explicitly first. Suspending does not stop
   Solder's deletion path, and an Application set to `DeleteManagedResources`
   would delete the workloads you just took over:

   ```sh
   kubectl -n <namespace> patch applications.solder.io <app> --type merge \
     -p '{"spec":{"deletionPolicy":"Orphan"}}'
   kubectl -n <namespace> get applications.solder.io <app> \
     -o jsonpath='{.spec.deletionPolicy}'   # must print Orphan
   kubectl -n <namespace> delete applications.solder.io <app>
   ```

6. **Drop Solder's leftover field ownership.** Where both controllers applied
   the same values, Server-Side Apply records `solder` as a co-owner, and a
   field removed from Git later would stay live. Clear the records on the
   migrated objects, as in [step 6 of the Flux guide](migrate-flux.md#6-drop-fluxs-leftover-field-ownership):

   ```sh
   kubectl get all,configmap,secret,ingress,serviceaccount,role,rolebinding,pvc \
     -n <namespace> -l sync.kuvryn.io/application=<app> -o name |
     xargs -I% kubectl -n <namespace> patch % --type merge \
       -p '{"metadata":{"managedFields":[{}]}}'
   ```

7. **Settle the Application:** set `spec.sync.conflictPolicy` back to `fail`,
   and turn on pruning and automatic sync if you want them.
8. **Update what reads the old names:** dashboards and alerts on `solder_*`
   metrics, notification receivers that verify `X-Solder-Signature`, scripts
   that call the `solder` CLI, and anything that selects on `solder.io/` labels.
9. **Remove Solder** once every Application has moved: `helm uninstall` the
   Solder release and delete the `solder.io` CRDs.

Upgrade notes for Solder releases up to 0.3.0 are in the
[changelog](https://github.com/azrtydxb/kuvryn-sync/blob/main/CHANGELOG.md) and in
the [0.3.0 upgrade notes](https://github.com/azrtydxb/kuvryn-sync/blob/v0.3.0/docs/upgrade.md).
