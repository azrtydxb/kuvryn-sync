# Deferred roadmap boundaries

Solder keeps the first release small and Kubernetes-native. These features are deliberately extension points, not hidden requirements.

## Progressive delivery

Canary and blue/green support should fit under explicit strategy fields and reuse Revision plan/apply/observe machinery. The current API leaves strategy room without adding rollout-specific CRDs.

## Multi-cluster

Multi-cluster operation should stay pull-based. A workload cluster should run Solder locally and reconcile its own Kubernetes API instead of requiring a central mandatory management plane.

## OCI desired-state source

OCI desired-state bundles can be added as another Repository source type with immutable digests. Git remains the first source adapter.

## Policy and supply chain

Policy, signature, SBOM, and provenance checks should run after render/plan and before apply. They must consume redacted plan and provenance data and must not require exposing Secret values.

## Notifications and webhooks

Notification sinks are optional integrations. They should consume bounded lifecycle events and status, not become a required broker.

## AI explanations

AI remains an optional explanation layer over redacted plans, health, graph, drift, and diagnosis. It must not be required for reconciliation and must never receive Secret values by default.
