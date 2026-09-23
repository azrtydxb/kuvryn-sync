# Notification sink configuration

Status: open
Created: 2026-09-23
Epic: lifecycle-notifications
Sprint: -

## Description

As an operator, I configure where an Application's lifecycle notifications go.

## Acceptance criteria

- [ ] A sink can be a generic HTTPS webhook (HMAC-signed body) or a Slack incoming webhook; URLs and keys come from Secrets.
- [ ] Sinks are a namespaced `NotificationSink` CRD (decided 2026-09-23); Applications reference sinks by name and select which lifecycle events to send.
- [ ] Invalid or unreachable sink configuration shows as a condition, never blocks reconciliation.

## Evidence

