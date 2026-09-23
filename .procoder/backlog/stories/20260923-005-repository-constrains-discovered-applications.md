# Repository constrains discovered Applications

Status: open
Created: 2026-09-23
Epic: impersonated-apply
Sprint: -

## Description

As a platform admin, I control which service account Applications discovered from `.solder.yaml` may use, so repository write access does not become cluster access.

## Acceptance criteria

- [ ] Repository spec can pin the service account for discovered Applications; a discovered Application that names a different one is rejected with a Repository condition.
- [ ] Discovered Applications without a service account inherit the Repository's pinned account.
- [ ] A controller test covers a `.solder.yaml` that tries to escalate by naming a privileged service account.

## Evidence

