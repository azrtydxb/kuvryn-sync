# Manual approval targets exact Revision

Status: done
Created: 2026-09-17
Epic: manual-approval-and-sync-policy
Sprint: 005-phase-3-ssa-apply-and-sync-policy

## Description

Manual approval targets exact Revision.

## Acceptance criteria

- [x] Automatic=false creates an AwaitingApproval Revision.
- [x] Approval applies the planned Revision even if Git advances later.

## Evidence

- Evidence: `internal/syncpolicy.CheckApproval` requires a non-empty exact Revision match before manual mutation can proceed. Tests reject missing and stale/wrong revision approvals.

## Correction

- Historical correction: previous evidence was helper/doc oriented; later all-gap closure added controller/runtime, image, installer, CI, KW validation, and product e2e evidence.

## Closure Evidence

- Closed after the all-gap closure pass: product paths now include controller-integrated render/plan/apply/prune/health/drift/retry/rollback handling, bounded metrics and optional OTel tracing, lifecycle Events, CLI operations, installer/CI polish, and regenerated API/manifests. Verification evidence is the final `make fmt test`, `procoder test`, `procoder lint`, `procoder security`, and `procoder check` gate run for this pass.
