# Managed kinds are watched dynamically

Status: open
Created: 2026-09-23
Epic: generic-resource-support
Sprint: -

## Description

As an operator, I expect drift on any managed kind — Certificates, VirtualServices, custom CRs — to trigger a reconcile, not just the six hard-coded built-ins.

## Acceptance criteria

- [ ] The Application controller starts metadata-only watches for each GVK it has applied, filtered by the Solder managed label.
- [ ] Watches are started once per GVK and survive controller restarts via inventory rebuild.
- [ ] An envtest test edits a managed custom resource and observes a reconcile enqueue for its Application.

## Evidence

