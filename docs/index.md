---
title: Solder
nav_order: 1
---

# Solder documentation

**Solder — GitOps that sticks.**

Solder is a small Kubernetes-native GitOps controller. It reconciles desired
state from Git into Kubernetes with deterministic plans, Server-Side Apply,
separate sync and health state, bounded Revision history, drift detection,
self-heal, pruning, and rollback.

The first public API is intentionally compact:

- **Repository**: where desired state comes from.
- **Application**: what to render, apply, observe, prune, and heal.
- **Revision**: what happened for one resolved deployment attempt.

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
- [Roadmap](roadmap.md)

## Design promises

- Kubernetes is the source of runtime truth: state is surfaced through CRDs,
  Conditions, Events, and metrics.
- Sync and health are different questions and are reported separately.
- Server-Side Apply is the mutation mechanism; conflicts fail by default.
- Secret material is redacted from status, plans, logs, CLI output, Events,
  metrics, and diagnostics.
- History is bounded so status remains operator-friendly.
- Rollback is normal reconciliation against a previous healthy Revision, not a
  special side channel.
