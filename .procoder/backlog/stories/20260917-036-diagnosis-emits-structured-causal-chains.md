# Diagnosis emits structured causal chains

Status: open
Created: 2026-09-17
Epic: deterministic-diagnosis
Sprint: 006-phase-4-graph-health-diagnosis

## Description

Diagnosis emits structured causal chains.

## Acceptance criteria

- [x] Common failures such as missing Secret produce resource-linked chains.
- [x] Same evidence powers status, CLI, and Events.
- [ ] Same evidence powers optional AI.

## Evidence

- Evidence: `internal/diagnosis.Build` combines graph edges and health results into deterministic structured causes with resource IDs, reason/message, dependencies, and symptoms. Tests cover causal output for an unhealthy workload.

## Correction

- Historical correction: previous evidence was helper/doc oriented; later all-gap closure added controller/runtime, image, installer, CI, KW validation, and product e2e evidence.

## Closure Evidence

- Closed after the all-gap closure pass: product paths now include controller-integrated render/plan/apply/prune/health/drift/retry/rollback handling, bounded metrics and optional OTel tracing, lifecycle Events, CLI operations, installer/CI polish, and regenerated API/manifests. Verification evidence is the final `make fmt test`, `procoder test`, `procoder lint`, `procoder security`, and `procoder check` gate run for this pass.

## Superseded by the 0.2.0 cleanup

`internal/diagnosis` was removed: nothing used it. `solder diagnose` (`internal/cli/plan.go`) reports from Application and Revision status instead; structured causal chains never shipped.

## Delivered

Rebuilt on 2026-09-24 and wired into the controller and CLI; the story stays
open only for the optional AI consumer. The product-specific consumer named
in the earlier criterion was dropped: Solder integrates through its public
status, Events and CLI.

- `internal/diagnosis/diagnosis.go`: `Build` descends from each unhealthy
  managed object to leaf evidence (container waiting reasons, with the last
  termination for `CrashLoopBackOff`; `Unschedulable`; a Pending claim; a
  missing ConfigMap, Secret, claim or ServiceAccount; a Service without ready
  endpoints; a failed Job) and returns at most 10 deduplicated causes, each
  with root resource, CamelCase reason, redacted bounded message and chain.
- Status: `status.diagnosis` on Application (`api/v1alpha1/application_types.go`,
  `DiagnosisCause`), set by `ApplicationReconciler.diagnose`
  (`internal/controller/application_diagnosis.go`) and cleared when Healthy.
- Events: a `Diagnosed` Warning Event names the first cause when the set of
  causes changes.
- CLI: `solder diagnose` prints the chains (`RenderDiagnosis`,
  `internal/cli/graph.go`).
- Tests: `internal/diagnosis/diagnosis_test.go` covers each evidence type,
  shared-cause deduplication and the cap; the envtest "Application
  diagnosis" (`internal/controller/application_diagnosis_test.go`) shows a
  Degraded Deployment diagnosed as Deployment, ReplicaSet, Pod, missing
  Secret; `TestDiagnoseEmitsAnEventWhenCausesChangeOrTheApplicationDegrades` and
  `TestDiagnosePrintsChains` cover Events and CLI. Each was checked to fail
  with the code it covers removed.
