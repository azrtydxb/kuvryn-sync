# Changelog

## Unreleased

- **Breaking:** `spec.source.render.helm.releaseName` must follow Helm's naming
  rule, a lowercase DNS subdomain of at most 53 characters, and the CRD
  enforces it. Applications whose release name broke the rule never rendered,
  because Helm refused the name. They now fail with `ValidationFailure` before
  any Revision is created; rename the release. On clusters without CRD
  validation ratcheting (before Kubernetes 1.30), rename it before deleting
  such an Application, or its finalizer cannot be removed. A `.solder.yaml`
  naming an invalid release fails discovery for its Repository.
- Fixed: a stale managed resource annotated `solder.io/prune: "disabled"`, or
  of a high-risk kind (Namespace, CustomResourceDefinition,
  PersistentVolumeClaim, PersistentVolume, Secret), failed the whole rollout
  with `PruneFailure`. Prune now skips such resources and never deletes them;
  the rest of the prune proceeds and the rollout completes. The Revision plan
  lists each skipped resource as `Unchanged` with a warning saying why, and a
  `PruneSkipped` Warning Event names them. They keep Solder's labels and stay
  in the inventory, and later Revisions do not try to delete them again.
- Fixed: the Application `Ready` condition was only ever set to `False`, so a
  recovered Application kept a stale failure that `solder diagnose` printed.
  `Ready` now turns `True` with reason `Healthy` when a rollout completes
  Synced and Healthy, and `False` with the failure's reason whenever a rollout
  fails, not only when reconciliation stops before planning. After an
  automatic rollback it is `False` with reason `RolledBack`.
- Fixed: `solder version` always printed `solder development`. Builds now embed
  the version: the Git tag for a release image, `sha-<commit>` otherwise, and
  `dev` for a plain `go build`. The manager logs it once at startup.
- Applications explain why they are not Healthy. When a managed resource is
  unhealthy, Solder builds a graph of its live ReplicaSets, Pods,
  EndpointSlices and the ConfigMaps, Secrets, claims, volumes and
  ServiceAccounts they refer to, read as the Application's service account,
  and records up to ten root causes in the new `status.diagnosis`, each with
  the chain from the managed resource down to the cause, such as Deployment,
  ReplicaSet, Pod, missing Secret. A `Diagnosed` Warning Event names the first
  cause when the causes change. The diagnosis is cleared once the Application
  is Healthy. For the deepest diagnosis the service account needs `list` on
  Pods, ReplicaSets and EndpointSlices and `get` on what they refer to;
  without it, diagnosis stops higher up and reconciliation is unaffected.
- `solder diagnose` prints those causal chains, and the new `solder graph`
  prints an Application's live resource graph as JSON or Graphviz DOT, read
  with your own credentials. `solder help` lists every command.
- Fixed: pulled Helm charts were cached forever. The hourly cache pruner now
  removes charts no render has used for a day.
- **Breaking:** the Revision `spec.provenance` field is removed, along with the
  unused product-specific integration helpers. Solder integrates with any tool
  through its public CRDs, status, Events, and CLI. Nothing in Solder read or
  set the field. Once the CRD is updated the field no longer appears on existing
  Revisions, and clients that still send it are rejected under strict field
  validation.
- Fixed: the Git source cache was never cleaned. Every commit Solder rendered
  stayed checked out on disk forever. Each replica now prunes, every hour,
  checkouts no Revision or Repository refers to and clones of deleted
  Repositories.
- Fixed: the Repository and Application controllers each used their own cache
  instance over the same directory, so two fetches of one repository could
  write the same clone at once. They now share one cache and its locks.
- Tracing works: with `OTEL_EXPORTER_OTLP_ENDPOINT` set, the manager exports a
  span per Application reconcile over OTLP gRPC, configured by the exporter's
  `OTEL_*` variables. Before, nothing installed a tracer provider, so the documented
  tracing produced nothing. Span errors are redacted like status messages. The
  Helm chart gains `extraEnv` to pass the variables.
- Credentials in URLs, such as `https://user:token@host/`, are now redacted
  from status messages, Events, CLI output, and spans. Before, only
  `password=...` style assignments and bearer tokens were.

## 0.2.0

_Each Application now applies as its own service account, and Solder reaches
parity with Flux and Argo CD for Helm repositories, SOPS, approvals,
notifications, hooks and waves, and image automation. Most changes below are
breaking; follow the [upgrade checklist](docs/upgrade.md) before installing._

- Solder now builds with Go 1.26, required by `golang.org/x/crypto` v0.57,
  which fixes two SSH channel deadlocks a Git server could trigger. Also
  bumped: `cel-go` v0.31 (JSON private field exposure), `golang.org/x/mod`
  v0.41 (sumdb verification), and the OTLP trace exporters.
- **Breaking:** Solder now reads, applies, and prunes each Application's
  resources by impersonating a service account, so tenant RBAC decides what an
  Application may change. Set `spec.serviceAccountName`, or start the manager
  with `--default-service-account` (Helm value `defaultServiceAccount`);
  Applications with neither are refused with `ServiceAccountRequired`.
- **Breaking:** Git credential Secrets must be labelled
  `solder.io/git-credentials: "true"`. Previously a Repository author could
  point `secretRef` at any Secret in the namespace and a Git URL they control,
  and Solder would send that Secret to it.
- The Helm chart's `image.tag` now defaults to `v<appVersion>`, the image of
  the chart's own release. The chart passes manager flags older images do not
  have, so do not pair it with an earlier release's image.
- **Breaking:** Kustomize renders refuse remote resources, bases, components,
  and generator or patch files (URLs, Git remotes, `?ref=` sources). Before,
  the documentation said they were not fetched, but Kustomize fetched `http(s)`
  resources from the controller with no timeout or size limit.
- **Breaking:** `render.helm.chart.version` must be an exact version. Ranges
  were downloaded once and then served from cache forever.
- **Breaking:** webhook receiver responses for unknown objects are now 401,
  like bad signatures, and callers are rate limited per remote address before
  authentication.
- Helm `valuesFiles` may be SOPS-encrypted. An encrypted values file without
  `spec.decryption` now fails the render instead of being merged as ciphertext.
- `solder approve` shows the plan digest and approves exactly that digest by
  sending `solder.io/approve-digest`; the webhook refuses it if the plan has
  changed and never stores it. Re-approving the same Revision after its
  approval went stale now works.
- Fixed: a chain of symlinks in a commit could resolve outside the checkout
  and let the checkout marker be written through it.
- Fixed: cached chart archives were shared across namespaces and credentials,
  so a namespace without credentials could render another namespace's private
  chart.
- Fixed: Kustomize decrypted every file in the repository, so another team's
  encrypted files broke unrelated Applications.
- Fixed: image write-back was never enabled in the shipped manager, and
  ImagePolicies rescanned in a loop because their own status writes triggered
  new scans.
- Fixed: notification delivery errors could include the sink URL, which for
  Slack is the credential, in Events; redirects are no longer followed.
- Fixed: Docker config keys such as `ghcr.io/` or
  `https://index.docker.io/v1/` did not match their registry.
- **Breaking:** the approval digest now hashes the redacted plan, so plans
  awaiting approval at upgrade show a new digest and must be approved again.
- **Breaking:** unknown `solder.io/hook` or Argo CD hook values fail the
  Revision with `ValidationFailure`; before, they were applied as ordinary
  objects. Helm delete and rollback hooks, and Argo CD `Skip`, `SyncFail`,
  `PreDelete`, and `PostDelete` hooks, are no longer applied; `solder.io/hook:
skip` is new.
- One manual approval covers every hook and wave of a Revision's rollout
  while the desired state is unchanged; the approval records
  `desiredStateHash`. Revisions carry a `RolloutComplete` condition.
- Fixed: with `selfHeal: false`, drift was reverted on the reconcile after it
  was reported.
- Fixed: a rollout paused by a dependency, approval wait, or failure was
  treated as drift, and its remaining waves were never applied.
- Fixed: a succeeded hook deleted by `ttlSecondsAfterFinished` was re-run, and
  a missing object counted as Healthy.
- Fixed: `DeploymentStarted` was emitted on every observation requeue.
- Fixed: `solder_application_reconcile_duration_seconds` measured the time since
  the manager started instead of each reconcile's own duration.
- Fixed: when Solder and another manager shared a field, the conflict could go
  unreported until apply failed.
- RBAC denials while reading, applying, or pruning fail the Revision with
  reason `Forbidden`. Kinds the service account may not list are skipped by
  pruning and reported with a `PruneInventoryIncomplete` Warning Event.
- The service account is part of the Revision identity, so switching accounts
  starts a fresh Revision instead of reusing one blocked by retry limits.
- `--default-service-account` is validated at startup.
- Sync hooks and waves: `solder.io/hook: pre-sync|post-sync` and
  `solder.io/sync-wave` (plus the Helm and Argo CD equivalents) order a
  rollout into groups that must each be Healthy before the next; failed hooks
  fail the Revision as `HookFailed`, `status.hooks` records them, and hooks are
  re-run for each new Revision. Helm test hooks are never applied.
- Fixed: a rollout still being observed was marked Healthy on the next
  reconcile once nothing was left to apply, without waiting for its
  resources to become Healthy.
- Fixed: list items in managedFields (such as containers keyed by name) were
  treated as owning the whole list, so fields the API server defaulted inside
  them looked like changes on every reconcile.
- Fixed: planning compared whole objects, so fields defaulted by the API
  server or owned by other field managers (an HPA's replicas, another tool's
  labels) looked like changes forever. Every Deployment re-applied on each
  reconcile, and any object touched by another manager showed as drift. The
  planner now reports only fields Solder declares, plus fields it owned and no
  longer declares, and reports conflicts per exact field and manager,
  including Update-operation managers.
- `spec.sync.conflictPolicy: adopt` takes ownership of fields another field
  manager holds (Server-Side Apply force), listing every field and previous
  manager in the plan; `fail` remains the default.
- Helm Applications can pull a pinned chart from an `https://` Helm
  repository or `oci://` registry (`render.helm.chart`), recording the
  archive digest on the Revision, and merge `valuesFrom` ConfigMaps/Secrets
  (read as the Application's service account) and inline `values`. Values
  from Secrets are masked in plans and render errors.
- ImagePolicies with `spec.webhook` scan immediately when the registry or CI
  calls `/hooks/imagepolicies/<namespace>/<name>` on the receiver (Bearer
  token, GitHub signature, or GitLab token).
- `Repository.spec.imageUpdate` commits ImagePolicy selections back to Git
  wherever Flux-compatible `{"$imagepolicy": "ns:name"}` markers appear,
  retrying when the branch moves and reporting `ImagesUpdated`.
- New `ImagePolicy` API: scans an OCI registry (with pull credentials from a
  Secret labelled `solder.io/registry-credentials: "true"`) and selects an
  image by semver range, tag pattern, or a fixed tag's digest, recording
  `image:tag@digest` in status. Registry requests are rate-limited per host.
- Push webhook receiver (`--webhook-receiver-bind-address`, Helm
  `webhookReceiver.enabled`): signed GitHub pushes and token-authenticated
  GitLab pushes for a Repository with `spec.webhook` trigger an immediate
  fetch, with size limits, per-Repository rate limiting, and a metric.
- SOPS decryption with age keys: `spec.decryption` points at a Secret
  labelled `solder.io/decryption-key: "true"`; the `yaml` and `kustomize`
  renderers decrypt SOPS files in memory with MAC verification, and encrypted
  files without `spec.decryption` fail instead of being applied as ciphertext.
  Uses the official sops library, which grows the manager binary by about 45 MB.
- `spec.dependsOn` orders Applications in a namespace: a dependent plans but
  applies only once its dependencies are Healthy at their desired revision,
  with the `DependenciesReady` condition explaining any wait or cycle.
- Lifecycle notifications: the new `NotificationSink` API (HMAC-signed
  webhook or Slack, destination in a Secret) and `spec.notifications` on
  Applications send `AwaitingApproval` (with the approve command), `Healthy`,
  `Failed`, and `RolledBack` once per transition. Delivery is queued, retried,
  and never blocks reconciliation; misconfiguration shows as the
  `NotificationsReady` condition.
- Fixed: the labels and annotation Solder stamps on applied objects were
  planned as drift on the next reconcile, so every Application re-applied
  forever and manual approvals went stale immediately after applying.
- Manual approvals are attributable and bound to the plan: a mutating
  admission webhook records the authenticated approver, time, and plan digest
  whenever `solder.io/approved-revision` changes, and reverts hand-edited
  records. Revisions carry `status.plan.digest`; Solder applies only when the
  approved digest still matches, otherwise the Revision returns to
  AwaitingApproval with an `ApprovalStale` Event. Applied Revisions record
  `status.approval`. **Breaking:** an `approved-revision` annotation without the
  webhook's record no longer applies anything.
- `solder history -o json` exports the audit trail; `solder approve` aliases
  `solder sync`.
- Git, Kustomize, and Helm now run in process (go-git, kustomize/api, the
  Helm v4 SDK). Fixed: the controller image never shipped kustomize or helm,
  so `kustomize` and `helm` Applications could not render in it. The image no
  longer contains git either.
- Rendering is confined to the checkout: symlinks pointing outside it are
  refused, Kustomize cannot load bases outside it, and Helm values files must
  lie inside it (previously `../` values paths could read controller files).
- **Breaking:** Repository URLs must be `https`, `http`, `ssh`, or `git`;
  filesystem paths are rejected. SSH remotes require `known_hosts` in the
  credentials Secret. Kustomize remote bases are not fetched, and Helm chart
  dependencies must be vendored into `charts/`.
- Helm charts render with `.Release.Namespace` set to the destination
  namespace instead of `default`.
- New cluster-scoped `HealthCheck` API: ordered CEL rules decide the health of
  a kind, falling back to kstatus when none matches. Rules run with a cost
  limit and fail closed as Progressing.
- **Breaking (install):** a validating webhook rejects HealthChecks whose
  expressions do not compile to a bool. cert-manager is now required: the raw
  manifests and the Helm chart create an Issuer and Certificate for the
  webhook. Install the new `healthchecks.solder.io` CRD with the others.
- Health for kinds without a dedicated rule follows kstatus conventions
  (`observedGeneration`, `Stalled`, `Reconciling`, `Ready`); objects without a
  status are Healthy. Jobs are Healthy when complete and Degraded when failed.
  Previously every other kind was `Unknown` and held the rollout until
  `health.timeout` triggered a failure or rollback; `Unknown` no longer holds a
  rollout.
- `status.managedKinds` records the kinds an Application manages. Pruning now
  covers objects of any kind, not just ConfigMaps, Secrets, Services,
  Deployments, StatefulSets, and DaemonSets.
- Drift on any managed kind is detected: immediately through a metadata-only
  watch when the controller may list/watch the kind, otherwise every
  `--drift-resync-interval` (Helm value `driftResyncInterval`, default 5m).
- Fixed: once retry limits were reached, every reconcile replaced the
  Revision's failure with `RetryBlocked`, hiding the real cause. The Revision
  now keeps its original failure and the Application's Ready condition reports
  `RetryBlocked` together with it.
- **Breaking:** Applications discovered from `.solder.yaml` run as the new
  Repository field `spec.applicationServiceAccountName` and may not name any
  other service account; when the Repository sets none, they may not set one
  and use the controller default. Git write access can no longer choose which
  service account Solder acts as.
- Rendered objects without a namespace are placed in `destination.namespace`
  only when the cluster (or a CustomResourceDefinition rendered alongside
  them) says their kind is namespaced. Previously any cluster-scoped kind
  outside a short built-in list, such as StorageClass, was given the
  destination namespace. Kinds the cluster does not know fail with a
  retryable `ValidationFailure`.
- The controller ClusterRole no longer grants any write access to managed
  resources, RBAC objects, or CRDs; it keeps list/watch on the kinds it
  watches for drift. Those watches are metadata-only and Secrets are read
  uncached, so Secret contents are never held in controller memory. The Helm
  chart role now matches the generated role (it previously lacked the
  permissions the watches need).
- `status.serviceAccountName` and the `solder apps` output show the
  impersonated service account.
- The controller role gains `impersonate` on service accounts.

## 0.1.11

- Added Repository `spec.applicationConfigPaths` for monorepo and moved `.solder.yaml` discovery.
- Supported multiple Applications per `.solder.yaml` file through the `applications:` list.
- Documented config-path validation, duplicate Application-name rejection, discovery annotations, and pruning behavior.
- Updated Helm/default install examples to the `v0.1.11` image tag.

## 0.1.10

- Added product-path E2E coverage for Repository, Application, Revision, and applied workload reconciliation.
- Made E2E setup idempotent and included E2E build-tag linting.
- Reconciled stale Procoder planning signals after MVP closure.
- Fixed local Procoder CLI version mismatch so finish review can run `procoder review`.
