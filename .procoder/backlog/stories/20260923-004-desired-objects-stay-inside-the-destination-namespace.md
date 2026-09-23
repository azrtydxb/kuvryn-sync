# Desired objects stay inside the destination namespace

Status: done 2026-09-23
Created: 2026-09-23
Epic: impersonated-apply
Sprint: -

## Description

As a platform admin, I rely on Solder rejecting rendered namespaced objects that target a namespace other than `destination.namespace`, as defence in depth behind RBAC.

## Acceptance criteria

- [x] Desired-state validation rejects a namespaced object whose namespace differs from the destination, with a plan-time error naming the object.
- [x] Objects with no namespace are defaulted to the destination (existing behaviour kept, covered by a test).
- [x] Cluster-scoped objects are passed through to RBAC unchanged; a test proves they are not silently re-namespaced.

## Evidence

- Rejection: already enforced by `validate.Desired` before any live read; `TestDesiredRejectsDestinationNamespaceViolation` now also asserts the error names the object (`other/runtime`).
- Defaulting: `defaults only namespaced objects to the destination, using the cluster's scope` asserts the ConfigMap lands in `payments`.
- Cluster-scoped: the same test asserts a StorageClass (not in the old hard-coded list) keeps no namespace; scope now comes from the RESTMapper or a rendered CRD's `spec.scope` (`internal/resource/scope.go`, `TestScopesResolveMappedAndRenderedKinds`). Treating every kind as namespaced makes the controller test fail (mutation checked). Unknown kinds fail with a retryable `ValidationFailure` (`TestScopesRejectUnknownKinds` and the controller test).
- Gates: `make test` passes, `make lint` 0 issues, `procoder check`/`lint`/`security` clean.
