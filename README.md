# Solder

**Solder — GitOps that sticks.**

Solder is a lightweight, deterministic, Kubernetes-native GitOps reconciliation
engine built in Go. It connects desired state in Git to actual state in
Kubernetes with first-class plans, Server-Side Apply, drift detection,
dependency-aware health, bounded revision history, and deterministic rollback.

Solder is intentionally a small operator, not a platform bundle. It does not
require Redis, PostgreSQL, a message broker, or a mandatory UI.

## Status

This repository is in Phase 0 foundation work. The initial controller-runtime
scaffold and `v1alpha1` CRD shapes exist for:

- `Repository` — desired-state source and authentication reference;
- `Application` — user-facing deployment and reconciliation policy;
- `Revision` — auditable record of a deployment attempt and bounded plan data.

## Project layout

```text
cmd/                    controller manager entry point
api/v1alpha1/           Solder public API types
internal/controller/    controller-runtime reconcilers
config/                 CRDs, RBAC, manager manifests, samples
test/e2e/               Kind-oriented end-to-end scaffold
solder-full-spec.md     product and engineering specification
```

## Development

Generate CRDs and deepcopy code:

```sh
make manifests generate
```

Run unit/controller tests:

```sh
make test
```

Build the controller manager:

```sh
make build
```

Install CRDs into the current Kubernetes context:

```sh
make install
```

Deploy the controller:

```sh
make deploy IMG=<registry>/solder:<tag>
```

## License

Licensed under the Apache License, Version 2.0.
