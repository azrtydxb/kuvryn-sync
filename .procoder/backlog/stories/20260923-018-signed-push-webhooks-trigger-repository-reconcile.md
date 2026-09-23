# Signed push webhooks trigger Repository reconcile

Status: open
Created: 2026-09-23
Epic: git-webhook-receiver
Sprint: -

## Description

As an operator, I point GitHub or GitLab push webhooks at Solder so changes deploy promptly.

## Acceptance criteria

- [ ] The manager exposes a receiver endpoint (separately enable-able) that validates GitHub HMAC and GitLab token signatures from a Secret.
- [ ] A valid push for a matching URL enqueues the Repository; invalid signatures return 401 and are counted in a metric.
- [ ] Receiver is rate-limited and payloads are size-bounded.
- [ ] Polling keeps working as a fallback.

## Evidence

