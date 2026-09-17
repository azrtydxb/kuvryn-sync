# Solder spec coverage matrix

This file maps `solder-full-spec.md` sections to tracked Procoder work.

| Spec section                                                                         | Tracking                                                                                                               |
| ------------------------------------------------------------------------------------ | ---------------------------------------------------------------------------------------------------------------------- |
| 1-4 Vision, principles, goals, installation                                          | phase-0-foundation, installation-docs-upgrades                                                                         |
| 5 Public API and core CRDs                                                           | api-schemas-and-samples                                                                                                |
| 6 Change Plan                                                                        | change-plan-model-and-cli, secret-redaction-and-plan-safety                                                            |
| 7 Reconciliation Engine                                                              | ssa-sync-engine, manual-approval-and-sync-policy, health-evaluators-and-rollout-observation                            |
| 8 Server-Side Apply and Ownership                                                    | ssa-sync-engine                                                                                                        |
| 9 Pruning and Drift                                                                  | ordering-and-pruning, drift-calculation-and-status, self-heal-conflict-safety                                          |
| 10 Resource Dependency Graph                                                         | resource-graph-inference                                                                                               |
| 11 Health and Diagnosis Engine                                                       | health-evaluators-and-rollout-observation, deterministic-diagnosis                                                     |
| 12 Apply Ordering                                                                    | ordering-and-pruning                                                                                                   |
| 13 Rollback                                                                          | rollback-planning-and-execution, failed-revision-retry-protection                                                      |
| 14 Manual Approval                                                                   | manual-approval-and-sync-policy                                                                                        |
| 15 CLI Specification                                                                 | operator-cli                                                                                                           |
| 16-19 Internal structure, controllers, concurrency, source cache                     | foundation-control-plane, git-source-and-cache, ha-scale-rate-limits                                                   |
| 20 Security Model                                                                    | ci-security-and-local-kind, secret-redaction-and-plan-safety                                                           |
| 21-22 Observability and Event Model                                                  | observability-events-metrics-otel, provenance-and-events-contract                                                      |
| 23 Dhole Integration                                                                 | dhole-observer-contract                                                                                                |
| 24 Kuvryn Integration                                                                | kuvryn-discovery-and-actions                                                                                           |
| 25 Optional AI Integration                                                           | policy-supply-chain-notifications-ai-roadmap                                                                           |
| 26 Failure Handling                                                                  | deterministic-diagnosis, failed-revision-retry-protection                                                              |
| 27 Suspension                                                                        | manual-approval-and-sync-policy                                                                                        |
| 28 Multi-Tenancy and RBAC                                                            | api-schemas-and-samples, ci-security-and-local-kind                                                                    |
| 29 Secrets and Diff Safety                                                           | secret-redaction-and-plan-safety                                                                                       |
| 30 Desired-State Validation                                                          | renderers-normalization-validation                                                                                     |
| 31 Revision Identity                                                                 | change-plan-model-and-cli, rollback-planning-and-execution                                                             |
| 32 Status Size and etcd Safety                                                       | bounded-history-and-gc                                                                                                 |
| 33 API Evolution                                                                     | api-schemas-and-samples, installation-docs-upgrades                                                                    |
| 34 Testing Strategy                                                                  | ci-security-and-local-kind, installation-docs-upgrades, ha-scale-rate-limits                                           |
| 35-39 Later phases/webhooks/events                                                   | progressive-delivery-roadmap, multi-cluster-and-oci-roadmap, policy-supply-chain-notifications-ai-roadmap              |
| 40-47 Install/delete/GC/idempotency/HA/rate/supply-chain/policy                      | installation-docs-upgrades, bounded-history-and-gc, ha-scale-rate-limits, policy-supply-chain-notifications-ai-roadmap |
| 48-60 UX, workflows, MVP, phases, DoD, guardrails, interfaces, data flow, philosophy | operator-cli, all milestones, solder-full-product plan                                                                 |
