# User-defined CEL health rules

Status: open
Created: 2026-09-23
Epic: generic-resource-support
Sprint: -

## Description

As an operator, I write a CEL expression for a kind whose status kstatus cannot interpret, so Solder reports its real health.

## Acceptance criteria

- [ ] Rules live in a cluster-scoped `HealthCheck` CRD (decided 2026-09-23), created with `kubebuilder create api`, with CEL expressions validated at admission.
- [ ] A rule matches by group/kind and returns Healthy, Progressing, or Degraded with an optional message; compile errors are reported, not ignored.
- [ ] Rules evaluate with a cost limit; an expensive rule fails closed with a clear reason.
- [ ] Unit tests cover a matching rule, a non-matching kind falling back to kstatus, and a compile error.

## Evidence

