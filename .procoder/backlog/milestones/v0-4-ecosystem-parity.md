# v0.4 Ecosystem parity

Status: open
Created: 2026-09-23

## Goal

Teams can move to Solder from Flux or Argo CD without rewriting their delivery: third-party Helm charts work, resources owned by another controller can be adopted deliberately, and ordered hooks cover migrations.

## Success state

- A chart from an HTTP or OCI Helm registry deploys with inline and referenced values.
- A workload previously managed by Argo CD or Flux is adopted by Solder following a documented guide.
- A pre-sync Job runs and gates apply.
