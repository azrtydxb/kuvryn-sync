# Delivery is bounded and redacted

Status: done 2026-09-23
Created: 2026-09-23
Epic: lifecycle-notifications
Sprint: -

## Description

As a security reviewer, I trust notifications never leak Secret data or stall the controller.

## Acceptance criteria

- [x] Payloads are built only from redacted status and plan summaries; a test asserts Secret values never appear.
- [x] Delivery has a timeout, bounded retries, and a metric for failures; a slow sink does not delay reconcile.
- [x] AwaitingApproval notifications include the command or link needed to approve.

## Evidence

- Bounded delivery: `internal/notify.Dispatcher` (manager Runnable) with a 1000-entry in-memory queue, 10s HTTP timeout, 3 attempts with backoff; `TestFailedDeliveryReportsAfterAllAttempts` (3 attempts, then OnFailure → `NotificationFailed` Event), `TestEnqueueNeverBlocks` (full queue drops instead of blocking; `NotificationDropped` Event), metric `solder_notification_deliveries_total{type,result}`.
- Redaction: messages are built only from status and plan summary and passed through `redact.String`; `redacts secrets from failure notifications` asserts `token=super-secret-token` never reaches the sink. Failure messages are already redacted upstream by `safeMessage`, so removing the notifier's own redaction alone does not fail this test; it is a second layer.
- Approve command: `sends one signed AwaitingApproval notification with the approve command` asserts the exact `solder approve ... --revision` command and that a steady AwaitingApproval state is not re-notified; removing the transition guard makes it fail (mutation checked).
- Gates: `make test`, `make lint` 0 issues, `procoder check`/`security` clean.
