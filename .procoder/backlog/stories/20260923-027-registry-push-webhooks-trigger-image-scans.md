# Registry push webhooks trigger image scans

Status: done 2026-09-23
Created: 2026-09-23
Epic: image-automation
Sprint: -

## Description

As an operator, I let the registry (or CI) notify Solder when an image is pushed, so a new build is picked up in seconds rather than at the next scan interval.

## Acceptance criteria

- [x] The webhook receiver from the git-webhook-receiver epic accepts signed image-push events and enqueues the matching image policy.
- [x] Unsigned or mismatched events are rejected with 401 and counted in a metric.
- [x] Interval scanning continues as a fallback.

## Evidence

- `ImagePolicy.spec.webhook.secretRef` plus the receiver route `POST /hooks/imagepolicies/{namespace}/{name}`: an authenticated request (Bearer token, GitHub signature, or GitLab token, all constant-time) stamps `solder.io/reconcile-requested-at`, which re-queues the ImagePolicy for an immediate scan.
- `TestRegistryWebhookRequestsAnImageScan`: wrong token 401 without stamping, policy without webhook 404, valid Bearer 202 with the annotation; accepting any Bearer token makes it fail (mutation checked). Requests are rate-limited per ImagePolicy and size-limited like Git webhooks, and counted in `solder_webhook_receiver_requests_total`.
- Interval scanning continues as the fallback; documented in operations.md.
- Gates: `make test`, `make lint` 0 issues, `procoder check` clean.
