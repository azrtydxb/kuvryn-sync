# What a human decided

Written 2026-09-25 12:19 UTC. procoder reads this
file to avoid asking a question twice; edit an answer here to change what
it believes. Reword the question and it will be asked again.

## (no longer asked)

Key: API group and label prefix replacing solder.io

Answer: sync.kuvryn.io

## (no longer asked)

Key: Approver identity capture

Answer: (2026-09-23) Validating admission webhook records the authenticated user.

## (no longer asked)

Key: Console frontend stack

Answer: React, Vite and TypeScript, embedded with embed.FS.

## (no longer asked)

Key: Console session lifetime

Answer: Until the ID token expires. No refresh tokens are stored.

## (no longer asked)

Key: Default username and group prefix for impersonated identities

Answer: None by default, configurable. system: identities are always refused.

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

Key: How console pages refresh

Answer: Poll the JSON API every 10 seconds.

## (no longer asked)

Key: How drift on non-built-in kinds is detected

Answer: (2026-09-23) Opt-in watches: watch a kind only when the controller has list/watch on it (SelfSubjectAccessReview); otherwise re-check drift on a periodic resync interval.

## (no longer asked)

Key: How existing Solder installs move over

Answer: A clean break, with no migration tooling. v0.3.x is the last solder release.

## (no longer asked)

Key: How the console follows Kubernetes RBAC

Answer: OIDC sign-in, with the console impersonating the user's username and groups for every read.

## (no longer asked)

Key: Image automation API shape

Answer: (2026-09-23) Compact: ImagePolicy plus Repository.spec.imageUpdate.

## (no longer asked)

Key: Image reference markers in Git

Answer: (2026-09-23) Flux-compatible setter comments.

## (no longer asked)

Key: Missing per-host agent rule files block commits

Answer: (2026-09-23) Stay uncommitted; the user will fix the gate.

## (no longer asked)

Key: Names for repository, module, CLI, image and chart

Answer: kuvryn-sync everywhere, except the CLI binary, which is ksync.

## (no longer asked)

Key: Namespace discovery for viewers who cannot list cluster-wide

Answer: Try cluster-wide first, then fall back to a namespace picker filled from the namespaces the user may list, with typed entry.

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

Key: Rebrand depth for Kuvryn Sync

Answer: Full rename: the API group, labels and annotations, the Go module and repository, the CLI, the image and the chart.

## (no longer asked)

Key: Refusal point for Applications without a service account

Answer: (2026-09-23) Refuse before planning: no Revision, no cluster reads, only a ServiceAccountRequired condition.

## (no longer asked)

Key: Repository-discovery file name

Answer: .ksync.yaml

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

Key: Version of the first Kuvryn Sync release

Answer: v0.4.0, continuing from solder v0.3.0.

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

Key: Where image bumps are pushed

Answer: (2026-09-23) A configurable branch (default: the Repository's branch); no PR integration.

## (no longer asked)

Key: Where the read-only console runs

Answer: A separate Deployment (ksync console) with its own ServiceAccount.

## (no longer asked)

Key: Whether the built SPA is committed

Answer: No. It is built in CI and the Dockerfile, and a plain go build embeds a placeholder page.

## (no longer asked)

Key: Who performs the GitHub repository rename and makes the GHCR package public

Answer: Claude does both. The GHCR visibility change is done through the web UI.

## (no longer asked)

Key: dependsOn scope

Answer: (2026-09-23) Same namespace only.

## (no longer asked)

Key: e2e fixture repository

Answer: Rename azrtydxb/solder-e2e-app to azrtydxb/kuvryn-sync-e2e-app and push .ksync.yaml.
