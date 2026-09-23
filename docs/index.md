---
title: Solder
nav_order: 1
---

# Solder documentation

**Solder — GitOps that sticks.**

Solder is a small Kubernetes-native GitOps controller for teams that need to
know who approved which change. It reconciles desired state from Git into
Kubernetes with deterministic plans that are approved against the
authenticated approver, applies each Application as its own service account,
and keeps bounded, auditable Revision history. It adds drift detection,
self-heal, pruning, rollback, sync waves and hooks, notifications, SOPS,
image automation, and Repository-driven Application discovery from
`.solder.yaml` files, without a database, broker, or mandatory UI.

The first public API is intentionally compact:

- **Repository**: where desired state comes from.
- **Application**: what to render, apply, observe, prune, and heal.
- **Revision**: what happened for one resolved deployment attempt.

Optional companions: **HealthCheck** (CEL health rules for a kind),
**NotificationSink** (where lifecycle notifications go), and **ImagePolicy**
(which image to run).

## Get started

1. [Install Solder](install.md)
2. [Follow the quickstart](quickstart.md)
3. [Understand the core concepts](concepts.md)
4. [Use the CLI](cli.md)

## Reference

- [API reference](api.md)
- [Operations guide](operations.md)
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
  metrics, and diagnostics.
- History is bounded so status remains operator-friendly.
- Rollback is normal reconciliation against a previous healthy Revision, not a
  special side channel.
