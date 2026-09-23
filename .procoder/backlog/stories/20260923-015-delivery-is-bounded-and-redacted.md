# Delivery is bounded and redacted

Status: open
Created: 2026-09-23
Epic: lifecycle-notifications
Sprint: -

## Description

As a security reviewer, I trust notifications never leak Secret data or stall the controller.

## Acceptance criteria

- [ ] Payloads are built only from redacted status and plan summaries; a test asserts Secret values never appear.
- [ ] Delivery has a timeout, bounded retries, and a metric for failures; a slow sink does not delay reconcile.
- [ ] AwaitingApproval notifications include the command or link needed to approve.

## Evidence

