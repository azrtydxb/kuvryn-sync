# Signed push webhooks trigger Repository reconcile

Status: done 2026-09-23
Created: 2026-09-23
Epic: git-webhook-receiver
Sprint: -

## Description

As an operator, I point GitHub or GitLab push webhooks at Solder so changes deploy promptly.

## Acceptance criteria

- [x] The manager exposes a receiver endpoint (separately enable-able) that validates GitHub HMAC and GitLab token signatures from a Secret.
- [x] A valid push for a matching URL enqueues the Repository; invalid signatures return 401 and are counted in a metric.
- [x] Receiver is rate-limited and payloads are size-bounded.
- [x] Polling keeps working as a fallback.

## Evidence

- `internal/receiver` serves `POST /hooks/{namespace}/{name}` when `--webhook-receiver-bind-address` is set (Helm `webhookReceiver.enabled`, with a Service); it runs on every replica (`TestReceiverRunsOnEveryReplica`).
- Auth: GitHub `X-Hub-Signature-256` HMAC and GitLab `X-Gitlab-Token`, constant-time, against `spec.webhook.secretRef` `token`. `TestSignedGitHubPushRequestsAFetch`, `TestGitLabTokenPushRequestsAFetch`; `TestReceiverRejects` covers bad signature, bad token, no signature (401), unknown repo (404), other repository URL (400), oversized payload (413), none of which stamp the Repository. Accepting any signature or any URL makes these fail (mutation checked).
- Enqueue: a valid push stamps `solder.io/reconcile-requested-at`, which re-queues the Repository through its watch; polling continues as the fallback.
- Limits: 1 MB body, per-Repository token bucket (10/s) created only for existing Repositories; `TestReceiverRateLimitsPerRepository` (429); metric `solder_webhook_receiver_requests_total{result}`.
- Real-cluster check (Helm chart on Kind, signed push via the Service) is scripted but not yet run: Docker Desktop failed to start (2026-09-23).
- Gates: `make test`, `make lint` 0 issues, `procoder check`/`security` clean.
