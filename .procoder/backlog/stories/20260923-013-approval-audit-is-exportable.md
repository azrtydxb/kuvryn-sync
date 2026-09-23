# Approval audit is exportable

Status: done 2026-09-23
Created: 2026-09-23
Epic: attributable-approvals
Sprint: -

## Description

As a compliance officer, I export approval and apply history for a period without scraping Events.

## Acceptance criteria

- [x] `solder history <app> --output json` includes approver, plan digest, applied and completed timestamps, and outcome.
- [x] Output is redacted with the central redaction package; a test proves Secret data does not appear.

## Evidence

- `solder history <app> -o json` (`RenderHistoryJSON`) exports revisions oldest first with approver, approval time, plan digest, start/completion times, outcome, and failure reason.
- `TestRenderHistoryJSONExportsApprovalsAndRedacts` asserts ordering, approval fields, and that `password=hunter2` in a failure message never appears (central `redact.String`).
