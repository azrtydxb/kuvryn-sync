# Application API separates sync and health

Status: done
Created: 2026-09-17
Epic: api-schemas-and-samples
Sprint: 001-phase-0-closeout-source-render-kickoff

## Description

Application API separates sync and health.

## Acceptance criteria

- [x] Application spec covers source, destination, sync, strategy, health, history, deletion policy, and suspension.
- [x] Status keeps sync and health states independent.

## Evidence

- Evidence: `api/v1alpha1/application_types.go` includes source/destination/sync/strategy/health/history/deletionPolicy/suspend. Status has independent `.status.sync.state` and `.status.health.state`; controller test asserts independent Unknown initialization.
