# Graph captures common built-in relationships

Status: done 2026-09-24
Created: 2026-09-17
Epic: resource-graph-inference
Sprint: 006-phase-4-graph-health-diagnosis

## Description

Graph captures common built-in relationships.

## Acceptance criteria

- [x] Deployment/ReplicaSet/Pod, Service/EndpointSlice, Ingress/Service, HPA/workload, PDB/pods, PVC/PV edges are inferred.
- [x] Unknown relationships do not fail reconciliation.

## Evidence

- Evidence: `internal/graph.Build` infers deterministic owner, Service selector, PVC mount, and ConfigMap/Secret dependency edges across unstructured Kubernetes objects. Tests cover all common edge types.

## Correction

- Historical correction: previous evidence was helper/doc oriented; later all-gap closure added controller/runtime, image, installer, CI, KW validation, and product e2e evidence.

## Closure Evidence

- Closed after the all-gap closure pass: product paths now include controller-integrated render/plan/apply/prune/health/drift/retry/rollback handling, bounded metrics and optional OTel tracing, lifecycle Events, CLI operations, installer/CI polish, and regenerated API/manifests. Verification evidence is the final `make fmt test`, `procoder test`, `procoder lint`, `procoder security`, and `procoder check` gate run for this pass.

## Superseded by the 0.2.0 cleanup

`internal/graph` was removed: nothing in the controller or CLI used it, so the product never shipped a resource graph. Reopen this story to build one that is wired in.

## Delivered

Rebuilt on 2026-09-24 and wired into the controller and CLI.

- `internal/graph/graph.go`: `Build` infers `Owns` (any ownerReference,
  matched across API versions), `Selects` (Service to Pods and workload
  templates, PodDisruptionBudget to Pods), `Endpoints` (Service to
  EndpointSlice by `kubernetes.io/service-name`), `Routes` (Ingress rule and
  default backends to Service), `Scales` (HPA to `scaleTargetRef`), `Binds`
  (PVC to PV), `Mounts` (Pod or workload to PVC), `Uses` (ConfigMaps and
  Secrets from `envFrom`, `env` value sources, `configMap`, `secret` and
  projected volumes, and `imagePullSecrets`, across init, regular and
  ephemeral containers) and `RunsAs` (ServiceAccount). A referenced object
  missing from the input is a node marked missing.
- Unknown kinds and malformed objects never error: `Build` has no error
  result and skips objects without an identity.
- `internal/graph/collect.go`: `Collect` reads the live descendants and
  references of an Application's managed objects, bounded, treating
  Forbidden and other read failures as not visible.
- Tests: `internal/graph/graph_test.go` has one test per edge family
  (`TestOwnerReferencesLinkOwnersToOwnedObjects`,
  `TestServiceLinksEndpointSlicesPodsAndWorkloads`,
  `TestIngressRoutesToRuleAndDefaultBackends`,
  `TestAutoscalerAndDisruptionBudgetTargets`, `TestStorageEdges`,
  `TestPodSpecReferencesConfigurationAndIdentity`), missing-node marking
  (`TestMissingNodesAndPresentReferences`) and
  `TestUnknownKindsAndInvalidObjectsNeverFail`;
  `internal/graph/collect_test.go` covers bounded collection and Forbidden
  reads. Each was checked to fail with the code it covers removed.
