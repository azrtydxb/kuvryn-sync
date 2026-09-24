---
title: CLI reference
nav_order: 6
---

# CLI reference

The controller manager binary also exposes small operator-facing CLI commands.
When invoked with Kubernetes manager flags, it starts the controller. When
invoked with a Solder subcommand, it talks to the current kubeconfig context.

Every command that reads the cluster takes `-n` or `--namespace`. It defaults
to `default`, not to the kubeconfig context's namespace. Flags may come before
or after the arguments.

## Read commands

List Applications:

```sh
solder apps -n default
solder applications --namespace default
```

List Repositories:

```sh
solder repos -n default
solder repositories --namespace default
```

Read one Repository:

```sh
solder repo get platform -n default
```

Read one Application:

```sh
solder get payments -n default
solder status payments -n default
```

List Application history:

```sh
solder history payments -n default
```

Export the audit trail, oldest first, with approver, plan digest, start and
completion times, and outcome. Failure messages are redacted:

```sh
solder history payments -n default -o json
```

Read one Revision summary:

```sh
solder revision payments-abc123 -n default
```

## Plan output

Print the newest Revision plan for an Application:

```sh
solder plan payments -n default
```

Render a plan from a saved Revision object:

```sh
kubectl get revision payments-abc123 -o yaml > revision.yaml
solder plan payments -f revision.yaml
```

Structured output is available:

```sh
solder plan payments -n default -o json
solder plan payments -n default -o yaml
```

Plan output is bounded and redacted. Secret values and sensitive fields must not
appear in CLI output.

## Mutation commands

Approve an exact Revision for a manual sync policy (`solder approve` is an
alias):

```sh
solder sync payments -n default --revision payments-abc123
```

The command prints the Revision's plan digest and approves exactly that
digest; if the plan changes before the request is admitted, it is refused and
you review `solder plan` again. Running it again after an `ApprovalStale`
Event re-approves the new plan. The approval is recorded under your own
Kubernetes identity; see [Manual approval](operations.md#manual-approval).

Request rollback to the latest healthy Revision:

```sh
solder rollback payments -n default
```

Request rollback to a specific Revision object:

```sh
solder rollback payments -n default --revision payments-abc123
```

Suspend or resume reconciliation:

```sh
solder suspend payments -n default
solder resume payments -n default
```

## Diagnosis

Explain why an Application is not Healthy. The command prints the `Ready`
condition when it is `False`, such as a `SourceFailure` or `RetryBlocked`, the
latest Revision failure, and every cause recorded in `status.diagnosis`, each
with its chain from the unhealthy managed resource down to the root cause. When
`Ready` repeats the Revision failure's reason and message, it is printed once,
as the failure:

```sh
solder diagnose payments -n default
```

```text
payments: health Degraded, sync OutOfSync
Failure: HealthFailure: One or more resources are degraded
Causes (1):

1. MissingSecret  Secret/payments/db
   Secret payments/db does not exist; Pod api-7d9f-x2k: CreateContainerConfigError: ...
   Deployment/payments/api
   └─ ReplicaSet/payments/api-7d9f
      └─ Pod/payments/api-7d9f-x2k
         └─ Secret/payments/db
```

See [Reading a diagnosis](troubleshooting.md#reading-a-diagnosis).

## Resource graph

Print the live resource graph of an Application: its managed resources, the
ReplicaSets, Pods and EndpointSlices below them, and the ConfigMaps, Secrets,
claims, volumes and ServiceAccounts they refer to:

```sh
solder graph payments -n default
solder graph payments -n default -o dot | dot -Tsvg > payments.svg
```

`-o json` (the default) prints sorted `nodes` and `edges`; `-o dot` prints
Graphviz DOT. A node marked `missing` is referenced but does not exist; one
marked `unreadable` could not be checked, and `unread` (a comment in DOT)
lists the lists that failed, such as `could not list Pods: forbidden`. See
[Resource graph and diagnosis](concepts.md#resource-graph-and-diagnosis) for
the edges.

The command reads the cluster with your own kubeconfig credentials, so it
shows only what you may read. It needs, in the Application's destination
namespace:

- `get` on the Application, in its own namespace;
- `list` on every kind in the Application's `status.managedKinds`;
- `list` on `replicasets`, `pods` and `endpointslices`;
- `get` on the objects they refer to: `configmaps`, `secrets`,
  `serviceaccounts`, `persistentvolumeclaims`, `services`, any
  HorizontalPodAutoscaler target, and cluster-scoped `persistentvolumes`.

Secrets, ConfigMaps and ServiceAccounts are listed and read as metadata only,
so their data never leaves the API server; Kubernetes RBAC still asks for the
`list` and `get` verbs on them. A kind you may not list is left out, and a
reference you may not read is marked `unreadable`.

`drift` currently aliases the Application read path:

```sh
solder drift payments -n default
```

## Help

```sh
solder help
```

Lists every command. An unknown command prints the same list.

```sh
solder version
```

Prints the version.

## Install helper

```sh
solder install
```

This prints the raw `kubectl` installation commands. It does not mutate a
cluster by itself.
