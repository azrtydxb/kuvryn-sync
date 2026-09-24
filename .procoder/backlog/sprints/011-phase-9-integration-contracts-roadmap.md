# Phase 9 integration contracts roadmap

Status: closed 2026-09-17
Created: 2026-09-17
Milestones: phase-9-integration-contracts, deferred-roadmap
Spec: solder-full-product

## Goal

Finish provider-neutral integration contracts for provenance, lifecycle events, Dhole, Kuvryn, source-to-pod lineage, and document deferred roadmap boundaries without expanding Solder into a management plane.

## Committed stories

- [x] `20260917-057-revision-provenance-is-provider-neutral.md` — provider-neutral provenance
- [x] `20260917-058-typed-lifecycle-events-are-stable.md` — typed lifecycle events
- [x] `20260917-059-dhole-observes-deployment-result-without-deploying.md` — Dhole observer contract
- [x] `20260917-060-kuvryn-can-discover-solder-objects.md` — Kuvryn discovery
- [x] `20260917-061-kuvryn-actions-use-public-api.md` — Kuvryn actions
- [x] `20260917-062-lineage-links-running-pods-to-source-commit.md` — source-to-pod lineage
- [x] `20260917-063-canary-and-blue-green-api-room-is-preserved.md` — progressive delivery roadmap
- [x] `20260917-064-multi-cluster-options-stay-pull-based.md` — multi-cluster roadmap
- [x] `20260917-065-oci-desired-state-source-is-designed.md` — OCI source roadmap
- [x] `20260917-066-policy-and-supply-chain-checks-are-staged-after-plan.md` — policy/supply-chain roadmap
- [x] `20260917-067-webhook-and-notification-sinks-remain-optional.md` — optional notifications roadmap
- [x] `20260917-068-ai-remains-optional-explanation-layer.md` — optional AI roadmap

## Execution plan

- [x] Add typed contract helpers for provenance, lifecycle events, Dhole observation, Kuvryn discovery/actions, and lineage annotations.
- [x] Add tests proving contracts stay public-API based and provider-neutral.
- [x] Add roadmap ADR/docs for deferred feature boundaries.
- [x] Run gates.

## Sprint acceptance

- [x] Integration contracts rely on public CRDs/status/events only.
- [x] Provenance remains provider-neutral.
- [x] Lineage annotations are stable and bounded.
- [x] Deferred roadmap features are documented as explicit extension points.
- [x] `procoder test` and `procoder check` pass with no blockers.

## Retro

The integration layer stayed clean because Dhole and Kuvryn contracts were modeled as public CRD/status/action shapes instead of controller internals.

Future roadmap implementation should continue adding adapters behind Repository/Revision/Application contracts rather than adding mandatory services.

Keep optional features optional: no broker, no central plane, and no AI required for reconciliation.

## Result

committed: 12
done: 12 (20260917-057-revision-provenance-is-provider-neutral, 20260917-058-typed-lifecycle-events-are-stable, 20260917-059-dhole-observes-deployment-result-without-deploying, 20260917-060-kuvryn-can-discover-solder-objects, 20260917-061-kuvryn-actions-use-public-api, 20260917-062-lineage-links-running-pods-to-source-commit, 20260917-063-canary-and-blue-green-api-room-is-preserved, 20260917-064-multi-cluster-options-stay-pull-based, 20260917-065-oci-desired-state-source-is-designed, 20260917-066-policy-and-supply-chain-checks-are-staged-after-plan, 20260917-067-webhook-and-notification-sinks-remain-optional, 20260917-068-ai-remains-optional-explanation-layer)
carried: 0

## Retro

<!-- What slowed us down this sprint. -->

<!-- What we change next sprint because of it. -->

<!-- One adaptation from this sprint worth keeping. -->

## Superseded: Solder is universal

The product-specific contracts (`internal/contracts`, Dhole and Kuvryn helpers) and the Revision `spec.provenance` field were removed. Solder integrates with any tool through its public CRDs, status, Events, and CLI.
