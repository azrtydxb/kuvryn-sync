# What a human decided

Written 2026-09-23 10:13 UTC. procoder reads this
file to avoid asking a question twice; edit an answer here to change what
it believes. Reword the question and it will be asked again.

## (no longer asked)

Key: Approver identity capture

Answer: (2026-09-23) Validating admission webhook records the authenticated user.

## (no longer asked)

Key: Discovered Applications when the Repository pins no service account

Answer: (2026-09-23) Discovered Applications may not set serviceAccountName; they use the controller default.

## (no longer asked)

Key: Escalation fixture for the Kind e2e test (story 006)

Answer: (2026-09-23) Push `escalation/clusterrolebinding.yaml` to github.com/azrtydxb/solder-e2e-app.

## (no longer asked)

Key: Formatting of controller-gen output

Answer: (2026-09-23) Prettier-format generated YAML after every `make manifests`.

## (no longer asked)

Key: Git credential Secret exfiltration via Repository

Answer: (2026-09-23) Opt-in label: Solder only reads Secrets labelled `solder.io/git-credentials: "true"`.

## (no longer asked)

Key: Health model for non-built-in kinds

Answer: (2026-09-23) kstatus plus user-defined CEL health rules.

## (no longer asked)

Key: How Solder learns about new image builds

Answer: (2026-09-23) Built-in image automation: scan registries, pick by policy, commit the new reference back to Git.

## (no longer asked)

Key: How Solder runs Git, Kustomize, and Helm

Answer: (2026-09-23) In-process for all three: kustomize/api (krusty), the Helm SDK, and go-git.

## (no longer asked)

Key: How drift on non-built-in kinds is detected

Answer: (2026-09-23) Opt-in watches: watch a kind only when the controller has list/watch on it (SelfSubjectAccessReview); otherwise re-check drift on a periodic resync interval.

## (no longer asked)

Key: Missing per-host agent rule files block commits

Answer: (2026-09-23) Stay uncommitted; the user will fix the gate.

## (no longer asked)

Key: Next step after the comparison analysis

Answer: (2026-09-23) Create backlog epics for all P0–P2 items.

## (no longer asked)

Key: No service account named anywhere

Answer: (2026-09-23) Refuse to apply; Application shows a condition.

## (no longer asked)

Key: Notification sink configuration

Answer: (2026-09-23) Namespaced NotificationSink CRD referenced by Applications.

## (no longer asked)

Key: Product positioning

Answer: (2026-09-23) Approval-gated GitOps for regulated teams.

## (no longer asked)

Key: Refusal point for Applications without a service account

Answer: (2026-09-23) Refuse before planning: no Revision, no cluster reads, only a ServiceAccountRequired condition.

## (no longer asked)

Key: SOPS decryption implementation

Answer: (2026-09-23) Official sops/v3 library with age keys only; accept its dependency tree.

## (no longer asked)

Key: Tenancy model for Application apply permissions

Answer: (2026-09-23) Flux-style impersonation.

## (no longer asked)

Key: Unknown health and rollout completion

Answer: (2026-09-23) Unknown does not block completion; it is reported only.

## (no longer asked)

Key: Webhook certificates for HealthCheck and approval admission webhooks

Answer: (2026-09-23) Require cert-manager for webhook certificates (kubebuilder default).

## (no longer asked)

Key: Where CEL health rules live

Answer: (2026-09-23) Cluster-scoped HealthCheck CRD.

## (no longer asked)

Key: Where image automation sits in the backlog

Answer: (2026-09-23) v0.3 Regulated delivery, as its own epic.

## (no longer asked)

Key: dependsOn scope

Answer: (2026-09-23) Same namespace only.
