# Inline and referenced Helm values

Status: open
Created: 2026-09-23
Epic: helm-chart-sources
Sprint: -

## Description

As an application team, I set Helm values inline or from ConfigMaps and Secrets.

## Acceptance criteria

- [ ] Helm spec supports inline `values` and `valuesFrom` ConfigMap/Secret references with a documented merge order.
- [ ] Values from Secrets are redacted in plan and status output; a test proves it.

## Evidence

