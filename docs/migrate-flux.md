---
title: Migrate from Flux
nav_order: 13
---

# Migrate a Flux Kustomization to Solder

This moves one Flux `Kustomization` to a Solder `Application` without
deleting or restarting the workloads it manages. Flux applies with
Server-Side Apply as `kustomize-controller`, so Solder must adopt those fields
explicitly.

The examples migrate the Kustomization `payments` in `flux-system`, which
deploys `./apps/payments` from the `GitRepository` `platform` into the
`payments` namespace.

## 1. Stop Flux from changing or pruning the workloads

Suspend the Kustomization and turn off pruning. Pruning must be off before
the Kustomization is deleted in step 5, or Flux deletes everything it applied.

```sh
kubectl -n flux-system patch kustomization payments --type merge \
  -p '{"spec":{"suspend":true,"prune":false}}'
```

## 2. Give Solder a service account in the destination namespace

```sh
kubectl -n payments create serviceaccount payments-deployer
kubectl -n payments create rolebinding payments-deployer \
  --clusterrole admin --serviceaccount payments:payments-deployer
```

## 3. Create the Repository and an adopting Application

Point Solder at the same Git repository and path. Keep `automatic: false` so
you can review the takeover before anything changes.

```yaml
apiVersion: solder.io/v1alpha1
kind: Repository
metadata:
  name: platform
  namespace: payments
spec:
  git:
    url: https://github.com/example/platform.git
    revision: main
---
apiVersion: solder.io/v1alpha1
kind: Application
metadata:
  name: payments
  namespace: payments
spec:
  serviceAccountName: payments-deployer
  source:
    repositoryRef:
      name: platform
    path: apps/payments
    render:
      type: kustomize
  destination:
    namespace: payments
  sync:
    automatic: false
    prune: false
    conflictPolicy: adopt
```

## 4. Review and approve the takeover

The plan lists every field Solder takes from `kustomize-controller`:

```sh
solder plan payments -n payments
solder approve payments -n payments --revision <revision-name>
```

Wait until the Solder Application is Synced and Healthy:

```sh
kubectl -n payments get applications.solder.io payments
```

## 5. Remove the Flux Kustomization

With pruning off (step 1), deleting it leaves the workloads in place:

```sh
kubectl -n flux-system delete kustomization payments
```

## 6. Drop Flux's leftover field ownership

Where Solder applied the same values Flux had, Server-Side Apply records both
as owners, and `kustomize-controller` stays listed after Flux is gone. Clear
the ownership records on the migrated objects so the next change does not
conflict with a manager that no longer exists:

```sh
kubectl get all,configmap,secret,ingress,serviceaccount,role,rolebinding,pvc \
  -n payments -l solder.io/application=payments -o name |
  xargs -I% kubectl -n payments patch % --type merge \
    -p '{"metadata":{"managedFields":[{}]}}'
```

This covers the workload kinds `all` expands to (such as Deployments,
StatefulSets, DaemonSets, Services and Jobs), ConfigMaps, Secrets, Ingresses,
ServiceAccounts, Roles, RoleBindings and PersistentVolumeClaims. If the
Solder Application manages other kinds, such as custom resources, list them
and add them to the command:

```sh
kubectl -n payments get applications.solder.io payments \
  -o jsonpath='{range .status.managedKinds[*]}{.apiVersion}{" "}{.kind}{"\n"}{end}'
```

## 7. Settle the Application

Switch back to the default conflict policy so future conflicts stop the
deployment again, and enable pruning and automatic sync if you want them:

```sh
kubectl -n payments patch applications.solder.io payments --type merge \
  -p '{"spec":{"sync":{"conflictPolicy":"fail","prune":true,"automatic":true}}}'
```

Flux's `kustomize.toolkit.fluxcd.io/*` labels remain on the objects; they are
harmless and disappear once removed from your manifests or objects.
