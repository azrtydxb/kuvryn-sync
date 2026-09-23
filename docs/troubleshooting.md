---
title: Troubleshooting
nav_order: 10
---

# Troubleshooting

## Controller is not ready

```sh
kubectl -n solder-system get pods
kubectl -n solder-system logs deployment/solder-controller-manager -c manager
kubectl -n solder-system describe deployment solder-controller-manager
```

Common causes:

- image pull secret missing for private registries;
- image architecture does not match cluster nodes;
- read-only filesystem without a writable `/tmp` mount;
- RBAC denied for managed resources.

## Repository is Failed

```sh
kubectl describe repo <name> -n <namespace>
kubectl get repo <name> -n <namespace> -o yaml
```

Check:

- Git URL is reachable from the cluster;
- branch, tag, or commit exists;
- referenced Secret exists in the same namespace and is labelled
  `solder.io/git-credentials: "true"`;
- credentials are valid and allowed to read the repository;
- every configured `spec.applicationConfigPaths` entry is repository-relative,
  unique, stays inside the repository, and is named `.solder.yaml`;
- discovered Application names are unique across all configured `.solder.yaml`
  files.

## Application is Planning or AwaitingApproval

```sh
kubectl describe app <name> -n <namespace>
solder history <name> -n <namespace>
solder plan <name> -n <namespace>
```

For manual approval policies, approve the exact Revision. An `ApprovalStale`
Event means the plan changed after approval; review `solder plan` and approve
again:

```sh
solder sync <application> -n <namespace> --revision <revision-name>
```

## Application is OutOfSync or Drifted

Check the latest plan:

```sh
solder plan <application> -n <namespace>
```

If self-heal is disabled, Solder reports drift but does not mutate live objects.
Enable `spec.sync.selfHeal` if automatic correction is intended.

## Application is Degraded

```sh
solder diagnose <application> -n <namespace>
kubectl describe app <application> -n <namespace>
kubectl get events -n <namespace> --sort-by=.lastTimestamp
```

Inspect the managed workload resources named in Revision plan or failure status.
Health timeouts are controlled by `spec.health.timeout` and failure behavior by
`spec.strategy.failurePolicy`.

## ServiceAccountRequired or Forbidden

`ServiceAccountRequired` means the Application sets no `spec.serviceAccountName`
and the manager has no `--default-service-account`. Set one of them.

`Forbidden` means the Application's service account may not read, apply, or
delete a resource. The failure message names the verb and resource. Check what
the account may do:

```sh
kubectl auth can-i --list -n <destination-namespace> \
  --as system:serviceaccount:<application-namespace>:<service-account>
```

Grant the missing permission, then push a new commit or switch the Application
to a service account that has it; retry limits otherwise keep the failed
Revision blocked. A `PruneInventoryIncomplete` Warning Event names kinds the
account may not list, whose managed objects Solder cannot prune.

## Server-Side Apply conflict

Solder fails conflicts by default. Inspect the failing field manager with:

```sh
kubectl get <kind> <name> -n <namespace> -o yaml --show-managed-fields
```

Resolve ownership intentionally: update the external manager, move the field out
of Solder's desired state, or recreate the resource under a clear owner. Solder
will not force-take ownership in `v1alpha1`.

## Helm or Kustomize render failure

Check that the desired-state repository contains the expected path and renderer
inputs:

- `render.type: yaml` expects Kubernetes YAML files under the path.
- `render.type: kustomize` expects a Kustomize root. Bases and resources must be
  in the same repository; remote bases are not fetched.
- `render.type: helm` expects a chart and optional values files inside the
  repository. Chart dependencies must be vendored into `charts/` (run
  `helm dependency build` and commit the result). `.Release.Namespace` is the
  Application's destination namespace.
- A checkout fails if the commit contains a symlink pointing outside it.

Use `solder diagnose` and controller logs for the deterministic failure reason.
