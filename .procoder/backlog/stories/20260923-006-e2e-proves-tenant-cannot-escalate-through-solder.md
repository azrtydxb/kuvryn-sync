# E2E proves tenant cannot escalate through Solder

Status: done 2026-09-23
Created: 2026-09-23
Epic: impersonated-apply
Sprint: -

## Description

As a maintainer, I have a Kind e2e test that fails if Solder ever applies an object the Application's service account could not create itself.

## Acceptance criteria

- [x] E2E creates a namespace-scoped tenant SA and an Application whose Git path contains a ClusterRoleBinding; the Revision fails `Forbidden` and no binding exists.
- [x] The same Application with only namespaced Deployments syncs Healthy.
- [x] Test runs in the existing `test-e2e` workflow.

## Evidence

- Fixture: `escalation/clusterrolebinding.yaml` pushed to github.com/azrtydxb/solder-e2e-app (commit daa6fd3, approved 2026-09-23); binds cluster-admin to the e2e deployer service account.
- Escalation: `should refuse to apply what the Application's service account may not` asserts the Revision reaches `Failed:Forbidden` and `clusterrolebinding solder-e2e-escalation` does not exist.
- Namespaced path: `should reconcile a GitHub-backed Solder Application end to end` now runs as `solder-e2e-deployer` (admin in solder-e2e only) and reaches Synced/Healthy.
- Run: `make test-e2e IMG=solder:e2e-local KIND_LOAD_IMAGE=true` on an isolated Kind cluster: 4 of 4 specs passed (90s); the cluster was deleted afterwards. The first run exposed that exhausted retries overwrote the Forbidden failure with RetryBlocked; fixed in the same change set.
- Tests live in test/e2e/e2e_test.go, which the existing `test-e2e` workflow runs.
