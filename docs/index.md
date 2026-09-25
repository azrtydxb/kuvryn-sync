---
title: Kuvryn Sync
nav_order: 1
---

# Kuvryn Sync documentation

**Kuvryn Sync — an Azrty product.**

Kuvryn Sync is a small Kubernetes-native GitOps controller for teams that need to
know who approved which change. It reconciles desired state from Git into
Kubernetes with deterministic plans that are approved against the
authenticated approver, applies each Application as its own service account,
and keeps bounded, auditable Revision history. It adds drift detection,
self-heal, pruning, rollback, sync waves and hooks, notifications, SOPS,
image automation, and Repository-driven Application discovery from
`.ksync.yaml` files, without a database, broker, or mandatory UI.

When an Application is not Healthy, Kuvryn Sync walks its live resource graph down
to the root cause, such as a missing Secret or a crash-looping Pod, and
records the chain in `status.diagnosis`; `ksync diagnose` and `ksync graph`
print it. An optional, read-only [web console](console.md) shows the same
state in a browser, signing people in with OIDC and reading the cluster as
them, so what each person sees follows their own Kubernetes RBAC. The Helm chart and raw manifests expose Prometheus metrics on `:8443`
(the manager's own default leaves them off), and OpenTelemetry traces are
exported over OTLP once an endpoint is configured.

The first public API is intentionally compact:

- **Repository**: where desired state comes from.
- **Application**: what to render, apply, observe, prune, and heal.
- **Revision**: what happened for one resolved deployment attempt.

Optional companions: **HealthCheck** (CEL health rules for a kind),
**NotificationSink** (where lifecycle notifications go), and **ImagePolicy**
(which image to run).

## Get started

1. [Install Kuvryn Sync](install.md)
2. [Follow the quickstart](quickstart.md)
3. [Understand the core concepts](concepts.md)
4. [Use the CLI](cli.md)
5. [Set up the web console](console.md)

## Reference

- [API reference](api.md)
- [Operations guide](operations.md)
- [Web console](console.md)
- [Security model](security.md)
- [Troubleshooting](troubleshooting.md)
- [Upgrade notes](upgrade.md)
- [Migrate from Flux](migrate-flux.md) and [from Argo CD](migrate-argocd.md)
- [Roadmap](roadmap.md)

## Design promises

- Kubernetes is the source of runtime truth: state is surfaced through CRDs,
  Conditions, Events, and metrics.
- Sync and health are different questions and are reported separately.
- Server-Side Apply is the mutation mechanism; conflicts fail by default.
- Each Application changes only what its service account may change.
- Secret material is redacted from status, plans, logs, CLI output, Events,
  metrics, traces, and diagnostics.
- History is bounded so status remains operator-friendly.
- Rollback is normal reconciliation against a previous healthy Revision, not a
  special side channel.
