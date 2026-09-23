# Approval records approver identity

Status: done 2026-09-23
Created: 2026-09-23
Epic: attributable-approvals
Sprint: -

## Description

As an auditor, I read a Revision and see which user or group approved it and when.

## Acceptance criteria

- [x] An admission webhook (decided 2026-09-23), scaffolded with `kubebuilder create webhook`, captures the authenticated requester of the approval and stores it in Revision status. It is a mutating webhook: recording the approver requires writing it.
- [x] `solder approve` and a raw annotation patch both record identity.
- [x] An approval without an identifiable requester is rejected.

## Evidence

- Webhook: `kubebuilder create webhook --kind Application --defaulting`; `ApplicationCustomDefaulter` stamps `approved-by/at/digest` from `admission.Request.UserInfo` when `approved-revision` changes and restores them otherwise; the controller copies them into `Revision.status.approval` at apply.
- Envtest through the API server: `records the authenticated approver and the plan digest they approved` (impersonated user alice), `reverts forged approval records` (fails without the restore, mutation checked), `rejects approving a Revision that has no plan`, `clears the record when the approval is withdrawn`; `rejects the approval` covers a request without a user.
- CLI: `solder approve` aliases `solder sync`; `TestSyncPatchCarriesOnlyTheApprovedRevision` proves the CLI sets only `approved-revision`, so CLI and raw patches are recorded identically by the webhook.
- Controller: `applies only the exact approved manual Revision` asserts `status.approval.approvedBy`; `ignores an approval without a recorded approver and digest`. Discovery strips approval annotations from `.solder.yaml`.
- Kind e2e `should apply a manual Application only after an attributed approval` passed (9/9 specs) after Docker was restored; its first run exposed that Solder's own management metadata was planned as drift after every apply, which made approvals go stale. Fixed and covered by `plans no changes on the reconcile after an apply`. Helm chart install on Kind: the mutating webhook rejects approving a missing Revision.
