---
title: Roadmap
nav_order: 12
---

# Deferred roadmap boundaries

Kuvryn Sync keeps the first release small and Kubernetes-native. These features are deliberately extension points, not hidden requirements.

## Progressive delivery

Canary and blue/green support should fit under explicit strategy fields and reuse Revision plan/apply/observe machinery. The current API leaves strategy room without adding rollout-specific CRDs.

## Multi-cluster

Multi-cluster operation should stay pull-based. A workload cluster should run Kuvryn Sync locally and reconcile its own Kubernetes API instead of requiring a central mandatory management plane.

## OCI desired-state source

OCI desired-state bundles can be added as another Repository source type with immutable digests. Git remains the first source adapter.

## Policy and supply chain

Policy, signature, SBOM, and build-attestation checks should run after render/plan and before apply. They must consume the redacted plan and the recorded source identity (commit, chart digest) and must not require exposing Secret values.

## AI explanations

AI remains an optional explanation layer over redacted plans, health, drift, and the deterministic `status.diagnosis` Kuvryn Sync already records. It must not be required for reconciliation and must never receive Secret values by default.

## Delivered since 0.2.0

The live resource graph and root-cause diagnosis (`status.diagnosis`,
`ksync diagnose`, `ksync graph`) and OpenTelemetry trace export over OTLP
are no longer deferred; they ship in the next release. See the
[changelog](https://github.com/azrtydxb/kuvryn-sync/blob/main/CHANGELOG.md).
