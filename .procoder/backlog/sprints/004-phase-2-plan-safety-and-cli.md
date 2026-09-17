# Phase 2 plan safety and CLI

Status: closed 2026-09-17
Created: 2026-09-17
Milestones: phase-2-plan
Spec: solder-full-product

## Goal

Finish Phase 2 plan usability and safety: expose operator-friendly plan output, centrally redact Secret changes, make destructive actions conspicuous, and detect SSA conflicts before apply.

## Committed stories

- [x] `20260917-022-cli-plan-prints-operator-friendly-diff.md` — CLI plan output
- [x] `20260917-023-secret-values-are-centrally-redacted.md` — Secret redaction
- [x] `20260917-024-destructive-changes-are-conspicuous.md` — destructive change visibility
- [x] `20260917-025-ssa-conflicts-are-detected-before-apply.md` — SSA conflict detection

## Supporting todo tasks

- [x] `.procoder/todo/20260917-task-05-implement-change-plan-persistence-and-cli-output.md`

## Execution plan

- [x] Add field-level plan changes for safe scalar paths.
- [x] Add central redaction for Secrets and other sensitive paths.
- [x] Add a minimal `solder` CLI command surface for `plan` output over stored Revision/plan data or local fixture input.
- [x] Make delete actions visually and structurally conspicuous in plan output.
- [x] Add conflict model placeholders/tests that classify managed-field conflicts before apply.
- [x] Run gates before closing.

## Sprint acceptance

- [x] Plan output has table/text and JSON/YAML-safe structures.
- [x] Secret values never appear in plan resources, CLI output, logs, or tests.
- [x] Delete actions are unmistakable in machine and human output.
- [x] SSA conflict detection has tested model coverage.
- [x] `procoder test` and `procoder check` pass with no blockers.

## Retro

The useful seam was keeping planner status data and CLI presentation separate via `internal/planoutput`, so redaction happens centrally before any operator-facing output format.

Next sprint should keep apply policy separate from this plan model and let SSA execution consume the same conflict/destructive markers rather than recomputing them.

Keep file-backed CLI tests for deterministic command output while cluster lookup behavior is still thin.

## Result

committed: 4
done: 4 (20260917-022-cli-plan-prints-operator-friendly-diff, 20260917-023-secret-values-are-centrally-redacted, 20260917-024-destructive-changes-are-conspicuous, 20260917-025-ssa-conflicts-are-detected-before-apply)
carried: 0

## Retro

<!-- What slowed us down this sprint. -->

<!-- What we change next sprint because of it. -->

<!-- One adaptation from this sprint worth keeping. -->
