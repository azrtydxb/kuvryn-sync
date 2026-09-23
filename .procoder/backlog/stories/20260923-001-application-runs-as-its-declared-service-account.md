# Application runs as its declared service account

Status: done 2026-09-23
Created: 2026-09-23
Epic: impersonated-apply
Sprint: -

## Description

As a platform admin, I set `spec.serviceAccountName` on an Application so Solder applies, prunes, reads, and drift-checks its resources with that service account's permissions only.

## Acceptance criteria

- [x] `ApplicationSpec` has an optional `serviceAccountName`; `make manifests generate` is clean.
- [x] Apply, prune, live read, and drift read use a client impersonating `system:serviceaccount:<app-ns>:<name>`; a unit or envtest test fails if any path uses the controller client instead.
- [x] An RBAC denial surfaces as a non-retryable `Forbidden` Revision failure with a redacted message naming the resource kind.
- [x] Plans still render and persist for review when the service account lacks permission; only apply is blocked.

## Evidence

- Field: `spec.serviceAccountName` (+ `status.serviceAccountName`) added in `api/v1alpha1/application_types.go`; `make manifests generate` regenerates the CRD with the new field and the role with `impersonate` on serviceaccounts.
- Impersonation: `internal/impersonate` builds per-account clients; `ApplicationReconciler.tenantClient` is used for the live read, prune inventory, SSA apply, prune deletes, health read, and `DeleteManagedResources` deletion. Mutation check: switching the applier back to `r.Client` makes `plans with the tenant account but cannot apply what its RBAC forbids` fail (tenant escalated); restored afterwards. The tenant read path was proven by a real envtest RBAC denial on `list secrets`, which led to skipping kinds the account may not list in the prune inventory.
- Forbidden: the same test asserts Revision failure reason `Forbidden`, `Retryable: false`, message naming `clusterrolebindings`, and that no ClusterRoleBinding exists.
- Plan persists: the same test asserts `status.plan.summary.create == 2` on the failed Revision (account can read, not write).
- `make test` passes (controller suite 65.1% coverage); `make lint-fix` reports 0 issues (under the go.mod toolchain); `procoder lint` 0 findings; `procoder security` 0 findings.
