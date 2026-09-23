# Desired objects stay inside the destination namespace

Status: open
Created: 2026-09-23
Epic: impersonated-apply
Sprint: -

## Description

As a platform admin, I rely on Solder rejecting rendered namespaced objects that target a namespace other than `destination.namespace`, as defence in depth behind RBAC.

## Acceptance criteria

- [ ] Desired-state validation rejects a namespaced object whose namespace differs from the destination, with a plan-time error naming the object.
- [ ] Objects with no namespace are defaulted to the destination (existing behaviour kept, covered by a test).
- [ ] Cluster-scoped objects are passed through to RBAC unchanged; a test proves they are not silently re-namespaced.

## Evidence

