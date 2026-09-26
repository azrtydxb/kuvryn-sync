---
title: CLI reference
nav_order: 6
---

# CLI reference

The controller manager binary also exposes small operator-facing CLI commands.
When invoked with Kubernetes manager flags, it starts the controller. When
invoked with a Kuvryn Sync subcommand, it talks to the current kubeconfig context.

Every command that reads the cluster takes `-n` or `--namespace`. It defaults
to `default`, not to the kubeconfig context's namespace. Flags may come before
or after the arguments.

## Read commands

List Applications:

```sh
ksync apps -n default
ksync applications --namespace default
```

The read commands print a table whose columns are aligned with spaces:

```text
NAME       SYNC              HEALTH   DESIRED       DEPLOYED      SERVICEACCOUNT
podinfo    AwaitingApproval  Unknown  a30f1c2e9b7d                podinfo-deployer
guestbook  Synced            Healthy  b939e830aae1  b939e830aae1  guestbook-deployer
```

List Repositories:

```sh
ksync repos -n default
ksync repositories --namespace default
```

Read one Repository:

```sh
ksync repo get platform -n default
```

Read one Application:

```sh
ksync get payments -n default
ksync status payments -n default
```

List Application history:

```sh
ksync history payments -n default
```

Export the audit trail, oldest first, with approver, plan digest, start and
completion times, and outcome. Failure messages are redacted:

```sh
ksync history payments -n default -o json
```

Read one Revision summary:

```sh
ksync revision payments-abc123 -n default
```

## Plan output

Print the newest Revision plan for an Application:

```sh
ksync plan payments -n default
```

The text output names the Revision object, its commit and its phase. A plan
awaiting approval ends with the exact command that approves it; it passes `-n`
when you gave one or the namespace is not `default`:

```text
Application: podinfo
Revision:    podinfo-b939e830aae1
Commit:      b939e830aae1c0ffee
Phase:       AwaitingApproval

0 changed
2 created
0 deleted
0 unchanged

Approve with: ksync sync podinfo -n ksync-demo --revision podinfo-b939e830aae1
```

Render a plan from a saved Revision object:

```sh
kubectl get revision payments-abc123 -o yaml > revision.yaml
ksync plan payments -f revision.yaml
```

Structured output is available:

```sh
ksync plan payments -n default -o json
ksync plan payments -n default -o yaml
```

Structured output has `application`, `revision` (the commit), `revisionName`,
`phase` and `plan`.

Plan output is bounded and redacted. Secret values and sensitive fields must not
appear in CLI output.

## Mutation commands

Approve an exact Revision for a manual sync policy (`ksync approve` is an
alias):

```sh
ksync sync payments -n default --revision payments-abc123
```

The command prints the Revision's plan digest and approves exactly that
digest; if the plan changes before the request is admitted, it is refused and
you review `ksync plan` again. Running it again after an `ApprovalStale`
Event re-approves the new plan. The approval is recorded under your own
Kubernetes identity; see [Manual approval](operations.md#manual-approval).

Request rollback to the newest known-good Revision, Healthy or deployed by an
earlier rollback, whose source revision is neither the desired nor the deployed
one. The command fails when there is none:

```sh
ksync rollback payments -n default
```

Request rollback to a specific Revision object:

```sh
ksync rollback payments -n default --revision payments-abc123
```

On an Application with manual sync, the rollback is also the approval: the
command says that it approves and deploys the target, and under whose
Kubernetes identity, and no `ksync sync` is needed. Kuvryn Sync approves the
target's plan as it re-plans it against the live state, so the approval
binds to the plan it applies:

```text
rollback requested for podinfo to podinfo-b939e830aae1 (a30f1c2e9b7d)
the request approves and deploys podinfo-b939e830aae1 as system:admin; no ksync sync is needed
holding dd50c3a1f2e4 once the rollback completes
```

With automatic sync the second line is `the request deploys <revision>`. When
no requester was recorded, as with webhooks disabled, the command says the
target awaits approval and prints the `ksync sync` command for it. See
[Rollback](concepts.md#rollback) for why the rollback approves rather than the
CLI approving a digest.

The command records the desired revision as the one rolled back from. Once the
rollback completes, that revision is held: it is not deployed again, even with
automatic sync, until a new commit arrives. Rolling back explicitly to a held
Revision lifts its hold. The target must belong to the Application. `ksync
approve` refuses a held Revision. See [Rollback](concepts.md#rollback).

Suspend or resume reconciliation:

```sh
ksync suspend payments -n default
ksync resume payments -n default
```

## Diagnosis

Explain why an Application is not Healthy. The command prints the `Ready`
condition when it is `False`, such as a `SourceFailure` or `RetryBlocked`, the
latest Revision failure, and every cause recorded in `status.diagnosis`, each
with its chain from the unhealthy managed resource down to the root cause. When
`Ready` repeats the Revision failure's reason and message, it is printed once,
as the failure:

```sh
ksync diagnose payments -n default
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
ksync graph payments -n default
ksync graph payments -n default -o dot | dot -Tsvg > payments.svg
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
ksync drift payments -n default
```

## Web console

```sh
ksync console --session-key-file /etc/ksync/session/session-key
ksync console --oidc-issuer-url https://dex.example.com --oidc-client-id ksync \
  --redirect-url https://console.example.com/auth/callback \
  --session-key-file /etc/ksync/session/session-key
```

Serves the read-only web console, which reads the cluster as each signed-in
user. People sign in with a Kubernetes token; with `--oidc-issuer-url` and
`--oidc-client-id`, which go together, OIDC is offered as well.
`ksync console --help` lists every flag. See [Web console](console.md) for
token sign-in, the Dex setup and the Helm chart values.

## Help

```sh
ksync help
```

Lists every command. An unknown command prints the same list.

```sh
ksync version
```

Prints the version.

## Install helper

```sh
ksync install
```

This prints the commands that install the release the binary was built from:
`kubectl apply` of the release's `install.yaml`, or the Helm chart from a
checkout of the release tag, with a note on pulling the image when the GHCR
package is private. It does not change the cluster. For `v0.6.2`:

```text
Install Kuvryn Sync v0.6.2. cert-manager must be running first.

With the release manifests:

  kubectl apply -f https://github.com/azrtydxb/kuvryn-sync/releases/download/v0.6.2/install.yaml

Or with the Helm chart, from a checkout of the release tag:

  git clone --depth 1 --branch v0.6.2 https://github.com/azrtydxb/kuvryn-sync.git
  cd kuvryn-sync
  kubectl apply -f config/crd/bases
  helm upgrade --install kuvryn-sync charts/kuvryn-sync \
    --namespace kuvryn-sync-system --create-namespace

The image ghcr.io/azrtydxb/kuvryn-sync:v0.6.2 may be private: create a pull Secret and set
image.pullSecrets with Helm, or patch the Deployment with the raw manifests.
See https://github.com/azrtydxb/kuvryn-sync/blob/v0.6.2/docs/install.md
```

A development build, whose version is `dev` or `sha-<commit>`, has no release
to install; it says so and points to [Install](install.md).
