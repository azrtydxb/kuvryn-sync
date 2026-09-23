---
title: CLI reference
nav_order: 6
---

# CLI reference

The controller manager binary also exposes small operator-facing CLI commands.
When invoked with Kubernetes manager flags, it starts the controller. When
invoked with a Solder subcommand, it talks to the current kubeconfig context.

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

The approval is recorded under your own Kubernetes identity; see
[Manual approval](operations.md#manual-approval).

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

Show the latest recorded deterministic failure for an Application:

```sh
solder diagnose payments -n default
```

`drift` currently aliases the Application read path:

```sh
solder drift payments -n default
```

## Install helper

```sh
solder install
```

This prints the raw `kubectl` installation commands. It does not mutate a
cluster by itself.
