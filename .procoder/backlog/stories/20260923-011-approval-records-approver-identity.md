# Approval records approver identity

Status: open
Created: 2026-09-23
Epic: attributable-approvals
Sprint: -

## Description

As an auditor, I read a Revision and see which user or group approved it and when.

## Acceptance criteria

- [ ] A validating admission webhook (decided 2026-09-23), scaffolded with `kubebuilder create webhook`, captures the authenticated requester of the approval and stores it in Revision status.
- [ ] `solder approve` and a raw annotation patch both record identity.
- [ ] An approval without an identifiable requester is rejected.

## Evidence

