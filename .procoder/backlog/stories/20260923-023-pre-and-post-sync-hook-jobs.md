# Pre- and post-sync hook Jobs

Status: done 2026-09-23
Created: 2026-09-23
Epic: sync-hooks-and-waves
Sprint: -

## Description

As an application team, I run a migration Job before apply and a smoke-test Job after health.

## Acceptance criteria

- [x] Resources annotated as pre-sync or post-sync hooks run at that stage; hook failure fails the Revision with the hook's name.
- [x] Hooks appear in the plan and in Revision status (`status.hooks`); completed hooks are kept until the next Revision deletes and re-runs them (documented policy).

## Evidence

- `ordering.Hook`/`Groups`: `solder.io/hook: pre-sync|post-sync`, Helm `pre-/post-install|upgrade`, Argo CD `PreSync`/`PostSync`; Helm test hooks are dropped before planning. `TestGroupsOrderHooksWavesAndKinds`.
- Apply: each reconcile walks pre-sync hooks → waves → prune → post-sync hooks and stops at the first group not yet Healthy (`applyAndObserve`).
- `runs a pre-sync hook first, records it, and replaces it for the next Revision`: sync objects wait while the hook runs, `status.hooks` shows it Progressing then Healthy, and the next Revision deletes and recreates it (new UID); skipping the replacement makes it fail (mutation checked).
- `fails the Revision naming a failed hook`: a Stalled hook fails the Revision as `HookFailed` with `Widget/migrate` and its message, and later groups are not applied.
- Gates: `make test`, `make lint` 0 issues, `procoder check` clean; Kind e2e 9/9 and the Flux migration check pass with these changes.
