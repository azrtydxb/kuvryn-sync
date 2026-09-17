---
title: Security model
nav_order: 9
---

# Security model

Solder is designed to keep sensitive material out of public operational
surfaces while still giving operators useful plans, events, and diagnostics.

## Secret handling

- Git credentials are referenced through Kubernetes Secrets.
- Secret values are not copied into Repository, Application, or Revision specs.
- Plan, status, log, Event, metrics, and CLI paths use centralized redaction.
- Desired Secret manifests are treated as sensitive even when rendered from Git.

## Server-Side Apply ownership

Solder mutates live objects with Server-Side Apply. The default and only current
conflict policy is `fail`, which prevents Solder from silently taking fields
owned by another manager.

## RBAC

The generated controller RBAC grants Solder access to its CRDs, Events, Secrets
needed for Git auth, and common Kubernetes resources it may manage. Review
`config/rbac/role.yaml` for the exact generated permissions before production
use.

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
