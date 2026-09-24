---
title: Troubleshooting
nav_order: 10
---

# Troubleshooting

## Controller is not ready

These commands use the Deployment name of the raw manifests. A Helm install
names it `<release>-solder`, such as `solder-solder` for the release `solder`.

```sh
kubectl -n solder-system get pods
kubectl -n solder-system logs deployment/solder-controller-manager -c manager
kubectl -n solder-system describe deployment solder-controller-manager
```

Common causes:

- image pull secret missing for private registries;
- image architecture does not match cluster nodes;
- image from a different release than the chart or manifests, which exits on
  an unknown flag such as `--drift-resync-interval`;
- read-only filesystem without a writable `/tmp` mount;
- Pod evicted for ephemeral storage, when the source cache outgrows the node;
  see [Source cache](operations.md#source-cache);
- RBAC denied for managed resources.

## No traces arrive

Tracing is off unless `OTEL_EXPORTER_OTLP_ENDPOINT` or
`OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` is set on the manager container; with
Helm, set it through `extraEnv`. Solder exports over OTLP gRPC only, usually
port 4317, not OTLP HTTP on 4318. A plain-text collector needs
`OTEL_EXPORTER_OTLP_INSECURE=true` or an `http://` endpoint. Export failures
appear in the manager log as `Failed to export traces`. See
[Metrics and tracing](operations.md#metrics-and-tracing).

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

`solder diagnose` prints the root causes Solder recorded in
`status.diagnosis`; see [Reading a diagnosis](#reading-a-diagnosis). Health
timeouts are controlled by `spec.health.timeout` and failure behavior by
`spec.strategy.failurePolicy`.

## Reading a diagnosis

Each cause in `status.diagnosis` names a root resource, a reason, a message,
and a chain. Read the chain from the top: the first entry is the managed
resource that is not Healthy, the last is the root cause, and the entries
between are how one leads to the other, such as the ReplicaSet and Pod between
a Deployment and a missing Secret. Fix the last entry; the others recover on
their own.

| Reason                                             | Root resource       | What to check                                                                                                                             |
| -------------------------------------------------- | ------------------- | ----------------------------------------------------------------------------------------------------------------------------------------- |
| `MissingSecret`, `MissingConfigMap`                | the missing object  | Create it, or fix the name in the Pod template; mark the reference `optional: true` if it may be absent.                                  |
| `MissingPersistentVolumeClaim`                     | the missing claim   | Create the claim or fix `claimName`.                                                                                                      |
| `MissingServiceAccount`                            | the missing account | Create the account or fix `serviceAccountName`.                                                                                           |
| `ImagePullBackOff`, `ErrImagePull`                 | the Pod             | The image name and tag, and the pull Secret; a missing pull Secret is reported as `MissingSecret`.                                        |
| `CrashLoopBackOff`                                 | the Pod             | The last exit code in the message, then `kubectl logs --previous`.                                                                        |
| `CreateContainerConfigError`                       | the Pod             | A key missing from a ConfigMap or Secret that exists.                                                                                     |
| `Unschedulable`                                    | the Pod             | Requests, node selectors, taints, and quotas named in the message.                                                                        |
| `ClaimPending`                                     | the claim           | The storage class and its provisioner.                                                                                                    |
| `NoReadyEndpoints`                                 | the Service         | Whether its selector matches ready Pods.                                                                                                  |
| `JobFailed`, `OOMKilled`, `ContainerFailed`        | the Job or its Pod  | The Job's Pods and their logs.                                                                                                            |
| `PodFailed`, or the Pod's reason such as `Evicted` | the Pod             | The Pod's status message: eviction, node pressure, or deadline.                                                                           |
| `FailedCreate`, `ProgressDeadlineExceeded`         | the workload        | The workload's conditions: quotas, admission, or a rollout that stopped progressing. `FailedCreate` is the usual `ReplicaFailure` reason. |
| `Missing<Kind>` for any other kind                 | the missing object  | A referenced object, such as a Service behind an Ingress, that does not exist.                                                            |

A cause whose chain is only the managed resource itself means Solder found no
deeper evidence; its reason is the resource's health verdict, such as
`ReplicasUnavailable` during an ordinary rollout. Solder then emits no
`Diagnosed` Event unless the Application is Degraded. When a list failed,
such a message ends with what was not visible, such as
`not visible: could not list Pods: forbidden`: the evidence may be there,
but the service account may not read it.

Solder reads the objects below managed resources as the Application's service
account. If the account may not list Pods or read Secrets, the diagnosis stops
higher up the chain, and a reference it could not check is never reported as
missing; see [Diagnosis permissions](operations.md#diagnosis-permissions).
`solder graph <application>` shows the same graph with your own credentials.

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
of Solder's desired state, recreate the resource under a clear owner, or, when
Solder should take over (for example while migrating), set
`spec.sync.conflictPolicy: adopt` and review the takeover in the plan.

## Helm or Kustomize render failure

Check that the desired-state repository contains the expected path and renderer
inputs:

- `render.type: yaml` expects Kubernetes YAML files under the path.
- `render.type: kustomize` expects a Kustomize root. Bases and resources must be
  in the same repository; a kustomization naming a URL or Git remote is refused
  with the reference in the error.
- `render.type: helm` expects a chart and optional values files inside the
  repository. Chart dependencies must be vendored into `charts/` (run
  `helm dependency build` and commit the result). `.Release.Namespace` is the
  Application's destination namespace.
- A checkout fails if the commit contains a symlink, or chain of symlinks,
  that resolves outside it, or an entry named `.solder-checkout`.
- `render.helm.chart.version` must be an exact version; ranges are refused.
- Without a `revision`, the remote must advertise a default branch (`HEAD`);
  otherwise set a revision.

Use `solder diagnose` and controller logs for the deterministic failure reason.
