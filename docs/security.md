---
title: Security model
nav_order: 9
---

# Security model

Solder is designed to keep sensitive material out of public operational
surfaces while still giving operators useful plans, events, and diagnostics.

## Secret handling

- Git credentials are referenced through Kubernetes Secrets. Solder only uses
  Secrets labelled `solder.io/git-credentials: "true"`, because whoever writes a
  Repository chooses both the Git URL and the Secret; without the label, any
  Secret in the namespace could be sent to an arbitrary Git server.
- Secret values are not copied into Repository, Application, or Revision specs.
- Plan, status, log, Event, metrics, and CLI paths use centralized redaction.
- Desired Secret manifests are treated as sensitive even when rendered from Git.

## Server-Side Apply ownership

Solder mutates live objects with Server-Side Apply. The default and only current
conflict policy is `fail`, which prevents Solder from silently taking fields
owned by another manager.

## RBAC and service account impersonation

Solder reads, applies, and prunes an Application's resources as a service
account in the Application's namespace, not as the controller. Kubernetes RBAC
therefore decides what each Application may change: an Application cannot
create a ClusterRoleBinding, or touch another team's namespace, unless its
service account could do so itself.

The service account is `spec.serviceAccountName`, or the manager's
`--default-service-account` (Helm value `defaultServiceAccount`) when the
Application sets none. The default is a name, looked up in each Application's
namespace. When neither is set, Solder refuses the Application with a
`ServiceAccountRequired` condition and neither reads nor changes its managed
resources.

An RBAC denial while reading, applying, or pruning fails the Revision with
reason `Forbidden`. Kinds the service account may not list are left out of
pruning and reported with a `PruneInventoryIncomplete` Warning Event. The
service account is part of the Revision identity, so switching an Application
to an account with the right permissions starts a fresh Revision.

An Application may name any service account in its own namespace. Creating
Applications in a namespace is therefore as powerful as the most privileged
service account there: grant it only to people who could already act as those
accounts.

A typical tenant grant binds the built-in `admin` ClusterRole in the
destination namespace only:

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: payments-deployer
  namespace: default
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: payments-deployer
  namespace: payments
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: admin
subjects:
  - kind: ServiceAccount
    name: payments-deployer
    namespace: default
```

Until the controller role is narrowed, `config/rbac/role.yaml` still grants
broad write access. Beyond its CRDs, Events, and Git-auth Secrets, Solder now
also needs `impersonate` on service accounts.

## Supply chain

Published controller images are built by GitHub Actions and pushed to GHCR:

```text
ghcr.io/azrtydxb/solder:<tag>
```

For production, pin immutable tags or digests and use your cluster's image
policy controls.

## Network access

The controller needs outbound access to configured Git remotes and access to the
Kubernetes API. Renderer execution is limited to the controller environment and
installed tools.

## Responsible disclosure

Please report security issues privately to the repository owner rather than
opening a public issue with exploit details.
