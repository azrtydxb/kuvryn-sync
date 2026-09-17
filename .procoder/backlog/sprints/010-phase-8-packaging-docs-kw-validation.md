# Phase 8 packaging docs KW validation

Status: closed 2026-09-17
Created: 2026-09-17
Milestones: phase-8-install-docs
Spec: solder-full-product

## Goal

Finish packaging and install documentation, validate image builds and cluster manifests on the KW cluster without local Docker, and preserve alpha upgrade compatibility where practical.

## Committed stories

- [x] `20260917-003-container-image-path-is-buildable.md` — KW BuildKit image build
- [x] `20260917-009-local-kind-development-loop-works.md` — KW cluster development loop replacement
- [x] `20260917-054-raw-manifests-and-helm-chart-install-solder.md` — manifests and Helm chart
- [x] `20260917-055-docs-explain-operation-and-api.md` — operation/API docs
- [x] `20260917-056-upgrade-tests-preserve-alpha-compatibility-where-practical.md` — alpha compatibility checks

## Supporting todo tasks

- [x] `.procoder/todo/20260917-task-16-package-and-document-release-artifacts.md`

## Execution plan

- [x] Add alpha Helm chart and operations docs.
- [x] Validate CRDs and chart with KW cluster server dry-run.
- [x] Build manager image through KW BuildKit only; never local Docker.
- [x] Add upgrade/compatibility verification notes.
- [x] Run gates.

## Sprint acceptance

- [x] Image build evidence comes from KW BuildKit, not local Docker.
- [x] Raw CRDs and chart render/apply with server dry-run on KW.
- [x] Docs explain operation, API, install, and KW development loop.
- [x] Alpha compatibility expectations are documented/tested where practical.
- [x] `procoder test` and `procoder check` pass with no blockers.

## Retro

KW BuildKit removed the local Docker blocker and produced image-build evidence without touching local Docker.

Future cluster validation should continue using KW server dry-run or isolated namespaces, not local Kind unless explicitly requested.

Keep the chart alpha and CRDs-first until CRD lifecycle/upgrade ownership is designed more fully.

## Result

committed: 5
done: 5 (20260917-003-container-image-path-is-buildable, 20260917-009-local-kind-development-loop-works, 20260917-054-raw-manifests-and-helm-chart-install-solder, 20260917-055-docs-explain-operation-and-api, 20260917-056-upgrade-tests-preserve-alpha-compatibility-where-practical)
carried: 0

## Retro

<!-- What slowed us down this sprint. -->

<!-- What we change next sprint because of it. -->

<!-- One adaptation from this sprint worth keeping. -->
