# Controller ClusterRole reduced to least privilege

Status: open
Created: 2026-09-23
Epic: impersonated-apply
Sprint: -

## Description

As a security reviewer, I read `config/rbac/role.yaml` and see only what Solder itself needs — its CRDs, Events, Git auth Secrets, and `impersonate` on serviceaccounts — not create/delete on ClusterRoles, ClusterRoleBindings, CRDs, and workloads.

## Acceptance criteria

- [ ] Managed-kind RBAC markers are removed from controllers; `make manifests` regenerates a role without workload, RBAC, or CRD mutation verbs.
- [ ] The Helm chart and `dist/install.yaml` match the generated role.
- [ ] docs/security.md describes the impersonation model and the required tenant RoleBinding pattern.

## Evidence

