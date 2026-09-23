# Notification sink configuration

Status: done 2026-09-23
Created: 2026-09-23
Epic: lifecycle-notifications
Sprint: -

## Description

As an operator, I configure where an Application's lifecycle notifications go.

## Acceptance criteria

- [x] A sink can be a generic HTTPS webhook (HMAC-signed body) or a Slack incoming webhook; URLs and keys come from Secrets.
- [x] Sinks are a namespaced `NotificationSink` CRD (decided 2026-09-23); Applications reference sinks by name and select which lifecycle events to send.
- [x] Invalid or unreachable sink configuration shows as a condition, never blocks reconciliation.

## Evidence

- API: `kubebuilder create api --kind NotificationSink --controller=false` (scaffold group mismatches fixed); `type: webhook|slack`, `secretRef` with `url` (https required) and `hmacKey` (required for webhook). Applications subscribe with `spec.notifications[].sinkRef/events`.
- Signing: `TestWebhookDeliveryIsSignedAndRetried` verifies `X-Solder-Signature` = HMAC-SHA256 of the body; the controller test's TLS sink rejects unsigned bodies.
- Condition: `reports a missing sink as a condition without blocking reconciliation` asserts `NotificationsReady=False/SinkInvalid` naming the sink while the Revision still reaches AwaitingApproval.
