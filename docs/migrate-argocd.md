---
title: Migrate from Argo CD
nav_order: 14
---

# Migrate an Argo CD Application to Solder

This moves one Argo CD `Application` to a Solder `Application` without
deleting or restarting the workloads it manages. Argo CD applies as
`argocd-controller` (client-side apply) or `argocd-controller` with
Server-Side Apply; either way Solder must adopt those fields explicitly.

The examples migrate the Argo CD Application `payments` in the `argocd`
namespace, which deploys `apps/payments` into the `payments` namespace.

## 1. Stop Argo CD from syncing, and from deleting on removal

Turn off automated sync and remove the resources finalizer. The finalizer
must be gone before step 5, or deleting the Argo CD Application deletes
everything it deployed.

```sh
kubectl -n argocd patch application payments --type json \
  -p '[{"op":"remove","path":"/spec/syncPolicy/automated"}]'
kubectl -n argocd patch application payments --type json \
  -p '[{"op":"remove","path":"/metadata/finalizers"}]'
```

If the Application has no automated sync policy, skip the first command.

## 2. Give Solder a service account in the destination namespace

```sh
kubectl -n payments create serviceaccount payments-deployer
kubectl -n payments create rolebinding payments-deployer \
  --clusterrole admin --serviceaccount payments:payments-deployer
```

## 3. Create the Repository and an adopting Application

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
      type: yaml # or kustomize / helm, matching the Argo CD source
  destination:
    namespace: payments
  sync:
    automatic: false
    prune: false
    conflictPolicy: adopt
```

## 4. Review and approve the takeover

```sh
solder plan payments -n payments
solder approve payments -n payments --revision <revision-name>
kubectl -n payments get application.solder.io payments
```

## 5. Remove the Argo CD Application

With the finalizer gone (step 1), deleting it leaves the workloads in place:

```sh
kubectl -n argocd delete application payments
```

## 6. Drop Argo CD's leftover field ownership

Where Solder applied the same values Argo CD had, both remain recorded as
owners. Clear the ownership records on the migrated objects so the next
change does not conflict with `argocd-controller`:

```sh
kubectl get all,configmap,secret,ingress -n payments \
  -l solder.io/application=payments -o name |
  xargs -I% kubectl -n payments patch % --type merge \
    -p '{"metadata":{"managedFields":[{}]}}'
```

Solder labels every object it applies with `solder.io/application`, so this
selects exactly the migrated objects whichever way Argo CD tracked them.

## 7. Settle the Application

```sh
kubectl -n payments patch application.solder.io payments --type merge \
  -p '{"spec":{"sync":{"conflictPolicy":"fail","prune":true,"automatic":true}}}'
```

Argo CD's tracking label or annotation (`app.kubernetes.io/instance` or
`argocd.argoproj.io/tracking-id`) remains on the objects and is harmless once
Argo CD no longer manages them.
