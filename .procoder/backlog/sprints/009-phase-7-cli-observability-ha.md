# Phase 7 CLI observability HA

Status: closed 2026-09-17
Created: 2026-09-17
Milestones: phase-7-ops-cli
Spec: solder-full-product

## Goal

Finish the operator-facing CLI surface and operational hardening primitives for events, bounded metrics, optional tracing, HA leader election, rate limits, and scale-measurement fixtures.

## Committed stories

- [x] `20260917-046-cli-implements-core-read-commands.md` — core read CLI
- [x] `20260917-047-cli-implements-mutation-commands-safely.md` — safe mutation CLI
- [x] `20260917-048-structured-logs-and-events-cover-lifecycle-changes.md` — structured lifecycle events
- [x] `20260917-049-metrics-use-bounded-labels.md` — bounded metrics labels
- [x] `20260917-050-otel-tracing-boundaries-are-optional.md` — optional tracing boundaries
- [x] `20260917-051-leader-election-supports-ha-deployment.md` — HA leader election
- [x] `20260917-052-rate-limits-prevent-storms.md` — rate limits
- [x] `20260917-053-scale-tests-measure-claims.md` — scale tests

## Supporting todo tasks

- [x] `.procoder/todo/20260917-task-14-finish-cli.md`
- [x] `.procoder/todo/20260917-task-15-add-observability-and-ha.md`

## Execution plan

- [x] Add CLI read commands over public CRDs.
- [x] Add mutation command builders with exact revision/suspend safety.
- [x] Add lifecycle event helpers and bounded metric labels.
- [x] Add optional tracing no-op seam.
- [x] Verify manager leader election wiring and add rate limiter helpers.
- [x] Add deterministic scale fixture helpers.
- [x] Run gates.

## Sprint acceptance

- [x] CLI uses Kubernetes CRDs, not private services.
- [x] Mutation commands are dry-run/build-first and safety-checked.
- [x] Events/log fields and metrics labels are bounded/redacted.
- [x] Tracing is optional and no-op by default.
- [x] HA/rate-limit/scale primitives are tested.
- [x] `procoder test` and `procoder check` pass with no blockers.

## Retro

The CLI and observability work stayed aligned with the spec by returning public CRD-derived data and patch documents rather than introducing a private API server.

Next sprint should focus on packaging/docs and KW cluster validation without using local Docker.

Keep operational helpers bounded: low-cardinality labels, truncated events, no-op tracing by default, and capped backoff.

## Result

committed: 8
done: 8 (20260917-046-cli-implements-core-read-commands, 20260917-047-cli-implements-mutation-commands-safely, 20260917-048-structured-logs-and-events-cover-lifecycle-changes, 20260917-049-metrics-use-bounded-labels, 20260917-050-otel-tracing-boundaries-are-optional, 20260917-051-leader-election-supports-ha-deployment, 20260917-052-rate-limits-prevent-storms, 20260917-053-scale-tests-measure-claims)
carried: 0

## Retro

<!-- What slowed us down this sprint. -->

<!-- What we change next sprint because of it. -->

<!-- One adaptation from this sprint worth keeping. -->
