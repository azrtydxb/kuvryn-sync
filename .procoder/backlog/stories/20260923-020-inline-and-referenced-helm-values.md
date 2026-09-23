# Inline and referenced Helm values

Status: done 2026-09-23
Created: 2026-09-23
Epic: helm-chart-sources
Sprint: -

## Description

As an application team, I set Helm values inline or from ConfigMaps and Secrets.

## Acceptance criteria

- [x] Helm spec supports inline `values` and `valuesFrom` ConfigMap/Secret references with a documented merge order.
- [x] Values from Secrets are redacted in plan and status output; a test proves it.

## Evidence

- `render.helm.values` (inline, schemaless) and `valuesFrom[]{kind ConfigMap|Secret, name, key}` read with the Application's service account; merge order chart defaults < valuesFiles < valuesFrom (in order) < inline, documented in operations.md.
- `pulls the pinned chart, records its digest, merges values in order, and hides Secret values`: the Secret's value wins over the ConfigMap's and appears only as REDACTED in the plan, while the inline `region` shows as `eu-west`. Values are templated into annotations so the test observes Solder's redaction rather than the planner's blanket masking of ConfigMap data; disabling `redactPlanValues` makes it fail (mutation checked). Render error messages are masked the same way.
- Gates: `make test`, `make lint` 0 issues, `procoder check` clean.
