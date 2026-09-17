# Secret values are centrally redacted

Status: done
Created: 2026-09-17
Epic: secret-redaction-and-plan-safety
Sprint: 004-phase-2-plan-safety-and-cli

## Description

Secret values are centrally redacted.

## Acceptance criteria

- [x] Secret data/stringData changes show key counts and REDACTED values only.
- [x] Redaction applies to plan status, CLI, logs, Events, and diagnostics.

## Evidence

- Evidence: central plan output redaction in `internal/planoutput.RedactDocument` hides Secret values and sensitive paths before text/json/yaml emission. Planner Secret diffs report only key counts with REDACTED values. Tests assert secret values do not appear in planner status conversion or CLI output.

## Correction

- Historical correction: previous evidence was helper/doc oriented; later all-gap closure added controller/runtime, image, installer, CI, KW validation, and product e2e evidence.

## Closure Evidence

- Closed after the all-gap closure pass: product paths now include controller-integrated render/plan/apply/prune/health/drift/retry/rollback handling, bounded metrics and optional OTel tracing, lifecycle Events, CLI operations, installer/CI polish, and regenerated API/manifests. Verification evidence is the final `make fmt test`, `procoder test`, `procoder lint`, `procoder security`, and `procoder check` gate run for this pass.
