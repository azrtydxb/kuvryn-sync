# Explicit adoption conflict policy

Status: done 2026-09-23
Created: 2026-09-23
Epic: ownership-adoption-and-migration
Sprint: -

## Description

As an operator migrating to Solder, I opt an Application into taking ownership of fields another manager holds.

## Acceptance criteria

- [x] `conflictPolicy` gains an explicit adopt/force value; the plan lists every field and previous manager being taken over.
- [x] Adoption requires approval when manual approval is enabled; `fail` stays the default.
- [x] Controller test covers adoption from a foreign field manager.

## Evidence

- `conflictPolicy: adopt` (CRD enum `fail;adopt`); the applier adds `client.ForceOwnership` only for adopt; `markConflictPolicy` records the effective policy on every planned conflict so `conflictFailure` fails only under `fail`.
- `lists the takeover in the plan, waits for approval, then owns the field`: a ConfigMap server-side-applied by `argocd-application-controller` shows the conflict `data.key` / manager / `adopt` in the plan, stays AwaitingApproval until approved, then holds Solder's value with the previous manager no longer owning `data.key`. Removing force ownership makes it fail (mutation checked).
- `still fails on the same conflict by default`: without the policy the Revision fails with `ConflictFailure` and the live value is untouched.
- Docs: security, concepts, troubleshooting, api.md, CHANGELOG. Gates: `make test`, `make lint` 0 issues, `procoder check` clean.
