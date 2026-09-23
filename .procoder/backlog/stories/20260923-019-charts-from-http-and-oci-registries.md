# Charts from HTTP and OCI registries

Status: done 2026-09-23
Created: 2026-09-23
Epic: helm-chart-sources
Sprint: -

## Description

As an application team, I deploy a third-party chart by repository URL, chart name, and version.

## Acceptance criteria

- [x] Helm render spec accepts an HTTP or `oci://` chart reference with a pinned version; registry credentials come from a Secret.
- [x] Chart downloads are cached by digest; the Revision records the resolved chart digest.
- [x] Charts with dependencies render: charts pulled from a repository carry their packaged dependencies; Git charts must vendor theirs (no render-time dependency fetching, which would let a Chart.yaml make the controller fetch arbitrary URLs).

## Evidence

- API: `render.helm.chart{repository (https:// or oci:// enforced by CRD pattern and code), name, version, secretRef}`; credentials must be labelled `solder.io/registry-credentials=true`.
- Pull: `internal/renderer/helm/pull.go` uses the Helm SDK `action.Pull` with private repository config, cache, credentials file, and plugins directory. `TestPullFromHTTPRepositoryCachesAndRenders` (basic auth, digest = sha256 of the archive, second pull served from cache, wrong credentials fail, plain http refused, rendering the pulled archive with values) and `TestPullFromOCIRegistry` (OCI chart artifact from the in-test registry).
- Digest: `pulls the pinned chart, records its digest...` asserts `Revision.status.chartDigest` equals the archive sha256 (it was first lost to a spec update overwriting status; fixed by recording it after the update).
- Dependencies: packaged charts include theirs; Git charts must vendor (`TestRenderReportsMissingDependencies` from story 029).
- Gates: `make test`, `make lint` 0 issues, `procoder check` clean; `procoder security` reports two info-level direct-write warnings in the test-only fake registry (JSON/blob responses, not HTML).
