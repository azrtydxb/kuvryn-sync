# Impersonated apply

Status: open
Created: 2026-09-23
Milestone: v0-2-tenancy-and-generic-resources

## Description

Every mutating and reading call Solder makes on behalf of an Application runs as that Application's service account, so Kubernetes RBAC — not Solder's own broad ClusterRole — decides what each tenant may change. Closes the escalation where any Application author (or `.solder.yaml` committer) can create ClusterRoleBindings through the controller. Decision recorded 2026-09-23: Flux-style impersonation, no Project CRD.
