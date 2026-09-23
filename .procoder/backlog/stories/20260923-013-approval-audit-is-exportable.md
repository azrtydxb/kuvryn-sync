# Approval audit is exportable

Status: open
Created: 2026-09-23
Epic: attributable-approvals
Sprint: -

## Description

As a compliance officer, I export approval and apply history for a period without scraping Events.

## Acceptance criteria

- [ ] `solder history <app> --output json` includes approver, plan digest, applied and completed timestamps, and outcome.
- [ ] Output is redacted with the central redaction package; a test proves Secret data does not appear.

## Evidence

