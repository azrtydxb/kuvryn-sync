# Repository constrains discovered Applications

Status: done 2026-09-23
Created: 2026-09-23
Epic: impersonated-apply
Sprint: -

## Description

As a platform admin, I control which service account Applications discovered from `.solder.yaml` may use, so repository write access does not become cluster access.

## Acceptance criteria

- [x] `Repository.spec.applicationServiceAccountName` pins the service account for discovered Applications; a discovered Application that names a different one is rejected with a Repository condition. When nothing is pinned, discovered Applications may not set one (decided 2026-09-23).
- [x] Discovered Applications without a service account inherit the Repository's pinned account, or the controller default when nothing is pinned.
- [x] A controller test covers a `.solder.yaml` that tries to escalate by naming a privileged service account.

## Evidence

- Pin: new `spec.applicationServiceAccountName` on Repository (CRD regenerated); `normalizeDiscoveredApplication` enforces it. Table `constrains the service account of discovered Applications`: inherits the pin, accepts naming it, uses the default when unpinned.
- Rejection: entries `rejects a different account than the pinned one` and `rejects any account when nothing is pinned` assert a Failed Repository with the reason in the condition message and that no Application was created. Disabling the check makes both fail (mutation checked).
- Docs: `.solder.yaml` examples no longer set serviceAccountName; README, quickstart, and api.md explain the Repository field; CHANGELOG marks it breaking.
- Gates: `make test` passes, `make lint` 0 issues, `procoder lint`/`security` clean.
