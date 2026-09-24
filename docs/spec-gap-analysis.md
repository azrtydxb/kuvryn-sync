---
title: Spec gap analysis
nav_order: 99
nav_exclude: true
---

# Solder Spec Gap Analysis

Status: the all-gap closure pass has no remaining pending spec/backlog gaps tracked in `.procoder`. Solder now has an integrated MVP product path through the Kubernetes controller, CLI, manifests, installer targets, CI workflows, and runtime observability seams.

## Closed product paths

- Public CRDs: `Repository`, `Application`, and `Revision` under `solder.io/v1alpha1`.
- Git source resolution with Kubernetes Secret auth and local cache.
- YAML, Kustomize, and Helm renderers through the Application reconcile path.
- Desired-state validation before mutation.
- Live desired-object reads and managed-resource inventory for prune candidates.
- Deterministic planner and bounded/redacted `Revision.status.plan` persistence.
- Exact manual approval by Revision object name.
- SSA apply with pre-apply conflict failure for default fail policy.
- Safe prune with opt-out/high-risk rejection.
- Managed-resource watches for supported built-in kinds.
- Drift detection and self-heal behavior through the Application reconcile path.
- Bounded history retention for labeled Revisions.
- Retry-loop protection with persisted attempts and backoff decisions.
- Failure-policy rollback target resolution, `RollingBack`/`RolledBack` phases, previous Revision tracking, rollback annotation lifecycle, and rollback Events.
- Health evaluation, timeout enforcement, and separate sync/health state.
- Application lifecycle Events for plan, approval, apply, healthy, rollback, and failure paths.
- Central redaction for status/event messages and plan output.
- Prometheus-compatible bounded metrics and optional OpenTelemetry tracing wired into the manager.
- CLI read/mutation/diagnostic/install commands.
- Raw installer and Helm rendering targets, server-side installer validation target, and CI e2e image build/push flow without local developer Docker.
- Deferred roadmap items are represented as design-compatible API room without adding mandatory CRDs, databases, brokers, UI, or scripting runtimes.

## Verification baseline

Use this baseline before future closure claims:

```sh
make fmt test
procoder test
procoder lint
procoder security
procoder check
```

Image and cluster validation should use repeatable CI or cluster environments with pullable images that match the target architecture.
