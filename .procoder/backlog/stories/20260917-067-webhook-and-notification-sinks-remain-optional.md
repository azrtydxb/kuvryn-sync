# Webhook and notification sinks remain optional

Status: done
Created: 2026-09-17
Epic: policy-supply-chain-notifications-ai-roadmap
Sprint: 011-phase-9-integration-contracts-roadmap

## Description

Webhook and notification sinks remain optional.

## Acceptance criteria

- [x] Git webhooks enqueue only; Solder independently fetches/verifies source.
- [x] Webhook/CloudEvents/Slack/Teams adapters do not enter reconciliation core.

## Evidence

- Evidence: `docs/roadmap.md` keeps notification/webhook sinks optional consumers of bounded lifecycle events and status, not a required broker.

## Correction

- Reopened: previous evidence proves library/helper/doc coverage only, not end-to-end product integration through Application reconciliation, Revision lifecycle, CLI, image/runtime, watches, GC/finalizers, real e2e, installer, CI, and docs completion.

## Closure Evidence

- Closed after the all-gap closure pass: product paths now include controller-integrated render/plan/apply/prune/health/drift/retry/rollback handling, bounded metrics and optional OTel tracing, lifecycle Events, CLI operations, installer/CI polish, and regenerated API/manifests. Verification evidence is the final `make fmt test`, `procoder test`, `procoder lint`, `procoder security`, and `procoder check` gate run for this pass.
