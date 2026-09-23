# Controller-wide default service account

Status: done 2026-09-23
Created: 2026-09-23
Epic: impersonated-apply
Sprint: -

## Description

As a cluster operator, I set a default service account on the manager so Applications that omit `serviceAccountName` still run with restricted permissions instead of the controller's own identity.

## Acceptance criteria

- [x] Manager flag (and Helm value) sets the default service account name, resolved in the Application namespace.
- [x] When neither the Application nor the flag names an account, Solder refuses before planning: no Revision is created and the cluster is not read, and the Application gets a `ServiceAccountRequired` Ready condition (decided 2026-09-23); a test covers it. With `DeleteManagedResources`, deletion orphans managed resources with a Warning Event instead of blocking forever.
- [x] The effective service account is shown in Application status and `solder apps` output.

## Evidence

- Flag: `--default-service-account` in `cmd/main.go`; Helm value `defaultServiceAccount` renders the arg only when set (`helm template` checked with and without the value).
- Refusal: `refuses an Application when no service account is configured` asserts the `ServiceAccountRequired` reason, Degraded state, and no Revisions; `orphans managed resources on delete when no service account is configured` asserts the Application is removed and the managed ConfigMap is kept.
- Visibility: `status.serviceAccountName` asserted in the escalation test; `solder apps` gains a SERVICEACCOUNT column, asserted in `TestRenderCoreReadCommands`.
