# Registry push webhooks trigger image scans

Status: open
Created: 2026-09-23
Epic: image-automation
Sprint: -

## Description

As an operator, I let the registry (or CI) notify Solder when an image is pushed, so a new build is picked up in seconds rather than at the next scan interval.

## Acceptance criteria

- [ ] The webhook receiver from the git-webhook-receiver epic accepts signed image-push events and enqueues the matching image policy.
- [ ] Unsigned or mismatched events are rejected with 401 and counted in a metric.
- [ ] Interval scanning continues as a fallback.

## Evidence

