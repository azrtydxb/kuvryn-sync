# Controller ClusterRole reduced to least privilege

Status: done 2026-09-23
Created: 2026-09-23
Epic: impersonated-apply
Sprint: -

## Description

As a security reviewer, I read `config/rbac/role.yaml` and see only what Solder itself needs — its CRDs, Events, Git auth Secrets, and `impersonate` on serviceaccounts — not create/delete on ClusterRoles, ClusterRoleBindings, CRDs, and workloads.

## Acceptance criteria

- [x] Managed-kind RBAC markers are removed from controllers; `make manifests` regenerates a role without workload, RBAC, or CRD mutation verbs.
- [x] The Helm chart matches the generated role; `dist/install.yaml` is regenerated from `config/` by `make build-installer` at release, since it pins the released image.
- [x] docs/security.md describes the impersonation model and the required tenant RoleBinding pattern.

## Evidence

- Markers: managed-kind RBAC markers replaced by `list;watch` on the six watched kinds; `make manifests` regenerates `config/rbac/role.yaml` with no write verbs outside `solder.io` and Events. `TestControllerRoleOnlyWritesSolderObjects` fails when HEAD~'s broad role is restored (mutation checked).
- Chart: `charts/solder/templates/rbac.yaml` rewritten to the generated rules; `TestHelmChartRoleMatchesGeneratedRole` fails on a one-rule change (mutation checked). dist/install.yaml deliberately not regenerated before release.
- Runtime: `Manager with the generated controller role` runs a real manager authenticated with a TokenRequest token for a service account bound only to the generated role; it syncs an Application (uncached labelled Git Secret read, impersonation, SSA apply) and recreates a deleted managed ConfigMap via the metadata watch. Removing the ConfigMap `WatchesMetadata` makes it fail (mutation checked).
- Docs: docs/security.md describes impersonation, the tenant RoleBinding pattern, and the narrowed controller role; CHANGELOG updated.
- Gates: `make test` passes, `make lint` 0 issues, `procoder check`/`lint`/`security` clean.
