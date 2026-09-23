# Helm renders in process

Status: done 2026-09-23
Created: 2026-09-23
Epic: in-process-sources-and-renderers
Sprint: -

## Description

As an application team, my `render.type: helm` Applications render in the published controller image, without a helm binary, with values files confined to the checkout.

## Acceptance criteria

- [x] `HelmRenderer` uses the Helm v4 SDK in client-only template mode instead of executing `helm template`, with the same release name and values-file semantics.
- [x] Values files outside the workspace are rejected (files beside the chart, e.g. `../envs/prod/values.yaml`, stay allowed); a unit test proves a `../` escape out of the workspace fails (the exec renderer allowed it).
- [x] Unit tests cover a chart with values overrides and a chart with a vendored dependency.

## Evidence

- `internal/renderer/helm` uses the Helm v4.1.4 SDK (`action.Install` with `DryRunClient`, `Replace`), the newest release on the project's Kubernetes 0.35 libraries; v4.2+ would force k8s 0.36/0.37 and controller-runtime 0.24. Values are read only from the workspace and merged with Helm's loader; no getters or URLs.
- `TestRenderChartWithValuesSubchartNamespaceAndHooks` covers a values override, a vendored subchart with its own values, `.Release.Namespace` = destination namespace, and hooks rendered like `helm template`; `TestRenderRejectsValuesOutsideWorkspace` fails without the containment check (mutation checked); symlink escape and missing-dependency tests added.
- oras-go upgraded 2.6.0 → 2.6.2 to clear 12 known vulnerabilities pulled in by the Helm SDK; `procoder security` 0 findings.
