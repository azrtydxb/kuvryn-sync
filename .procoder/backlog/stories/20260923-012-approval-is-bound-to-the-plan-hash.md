# Approval is bound to the plan hash

Status: open
Created: 2026-09-23
Epic: attributable-approvals
Sprint: -

## Description

As an approver, I know my approval cannot be reused for a different change that arrives under the same Revision.

## Acceptance criteria

- [ ] Revision status carries a plan digest; approval stores the digest it approved.
- [ ] If the rendered plan changes after approval, apply is blocked and the Revision returns to AwaitingApproval with an Event.
- [ ] Controller test covers a plan change between approval and apply.

## Evidence

