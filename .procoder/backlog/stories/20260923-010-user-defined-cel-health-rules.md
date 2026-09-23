# User-defined CEL health rules

Status: done 2026-09-23
Created: 2026-09-23
Epic: generic-resource-support
Sprint: -

## Description

As an operator, I write a CEL expression for a kind whose status kstatus cannot interpret, so Solder reports its real health.

## Acceptance criteria

- [x] Rules live in a cluster-scoped `HealthCheck` CRD (decided 2026-09-23), created with `kubebuilder create api`, with CEL expressions validated at admission.
- [x] A rule matches by group/kind and returns Healthy, Progressing, or Degraded with an optional message; compile errors are reported, not ignored.
- [x] Rules evaluate with a cost limit; an expensive rule fails closed with a clear reason.
- [x] Unit tests cover a matching rule, a non-matching kind falling back to kstatus, and a compile error.

## Evidence

- API: `kubebuilder create api --kind HealthCheck --namespaced=false --controller=false` then spec with group, kind, ordered rules (expression, state, message); scaffold group/path mismatches (`core.solder.io`) fixed in roles, CRD kustomization, and webhook marker.
- Admission: `kubebuilder create webhook --programmatic-validation`; validator compiles every rule with `health.CompileRule`. `is enforced by the API server at admission` (envtest) — which caught a scaffold webhook path the server could not reach; Kind e2e `should reject a HealthCheck with an invalid CEL rule at admission` (7/7 specs passed); Helm chart install on Kind with cert-manager rejects an invalid and admits a valid HealthCheck.
- Evaluation: `TestHealthCheckRulesDecideMatchingKinds` (match, kstatus fallback, other kinds unaffected), `TestHealthCheckCompileErrorsAreReported` (syntax, non-bool, NewEvaluator names the rule), `TestHealthCheckOverCostLimitFailsClosed` (Progressing/HealthCheckFailed mentioning cost). Controller `judges health with a HealthCheck rule for its kind` fails when the controller ignores HealthChecks (mutation checked).
- cert-manager required (decided 2026-09-23): config/default enables webhook + certmanager; Helm chart gains Service, Issuer, Certificate, ValidatingWebhookConfiguration, and the cert mount. Docs: api.md, concepts, install, quickstart, README, CHANGELOG.
- Gates: `make test`, `make lint` 0 issues, `procoder check` clean.
