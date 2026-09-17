# Samples apply against generated CRDs

Status: done
Created: 2026-09-17
Epic: api-schemas-and-samples
Sprint: 001-phase-0-closeout-source-render-kickoff

## Description

Samples apply against generated CRDs.

## Acceptance criteria

- [x] Repository/Application/Revision samples validate with generated OpenAPI schemas.
- [x] Samples reflect the product spec examples.

## Evidence

- Evidence: `config/samples/core_v1alpha1_{repository,application,revision}.yaml` use `solder.io/v1alpha1`; `make manifests` regenerated CRDs successfully and `bin/kustomize build config/default` passed earlier.
