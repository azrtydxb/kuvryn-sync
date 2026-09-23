# E2E proves tenant cannot escalate through Solder

Status: open
Created: 2026-09-23
Epic: impersonated-apply
Sprint: -

## Description

As a maintainer, I have a Kind e2e test that fails if Solder ever applies an object the Application's service account could not create itself.

## Acceptance criteria

- [ ] E2E creates a namespace-scoped tenant SA and an Application whose Git path contains a ClusterRoleBinding; the Revision fails `Forbidden` and no binding exists.
- [ ] The same Application with only namespaced Deployments syncs Healthy.
- [ ] Test runs in the existing `test-e2e` workflow.

## Evidence

