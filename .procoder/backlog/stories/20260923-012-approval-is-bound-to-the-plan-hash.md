# Approval is bound to the plan hash

Status: done 2026-09-23
Created: 2026-09-23
Epic: attributable-approvals
Sprint: -

## Description

As an approver, I know my approval cannot be reused for a different change that arrives under the same Revision.

## Acceptance criteria

- [x] Revision status carries a plan digest; approval stores the digest it approved.
- [x] If the rendered plan changes after approval, apply is blocked and the Revision returns to AwaitingApproval with an Event.
- [x] Controller test covers a plan change between approval and apply.

## Evidence

- `status.plan.digest` = sha256 of the desired-state hash and the full redacted plan; the webhook stamps the digest at approval time and `manualApproval` requires it to match.
- `requires a new approval when the plan changes after approval` changes the rendered state after approval: nothing is applied, the Revision returns to AwaitingApproval, and an `ApprovalStale` Event is emitted. Dropping the digest check makes the approval tests fail (mutation checked).
- Kind e2e `should apply a manual Application only after an attributed approval` is written but not yet run: Docker Desktop's storage returned I/O errors (2026-09-23) and needs a restart.
