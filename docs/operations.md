---
title: Operations
nav_order: 8
---

# Operations guide

Kuvryn Sync is installed from generated CRDs plus the controller manifests in
`config/` or the alpha Helm chart in `charts/kuvryn-sync`.

## Web console

The chart installs the optional read-only web console with
`console.enabled=true`. It needs an OIDC issuer, such as Dex, and a viewer
ClusterRole for the people who use it. See [Web console](console.md) for the
setup, every value, and troubleshooting.

## Day-two commands

Core commands use Kubernetes CRDs directly:

- `ksync apps -n <namespace>` lists Applications.
- `ksync repos -n <namespace>` lists Repositories.
- `ksync plan <application>` reads Revision status and prints redacted plan
  output.
- `ksync history <application>` lists retained deployment attempts.
- `ksync diagnose <application>` prints the latest deterministic failure and
  the causal chains in `status.diagnosis`.
- `ksync graph <application>` prints the live resource graph as JSON or DOT.
- `ksync rollback <application>` requests rollback to the newest known-good
  Revision other than the desired and deployed ones, and holds the revision
  rolled back from.

Application, Repository, and Revision status remain the public integration API.
Mutation helpers update public CRDs and require exact Revision approval where
applicable.

## Manual approval

With `spec.sync.automatic: false`, Kuvryn Sync plans each Revision and waits. To
approve, run `ksync approve`, which shows the plan digest and approves exactly
that plan. With `kubectl annotate`, set the Application annotation
`sync.kuvryn.io/approved-revision` to the Revision name and, to bind the approval
to the plan you reviewed, `sync.kuvryn.io/approve-digest` to its
`status.plan.digest`; the webhook refuses the request if the plan has changed
since, and never stores `approve-digest`. Setting it again re-approves the same
Revision. Anyone allowed to
update the Application can approve; use RBAC to decide who that is.

Kuvryn Sync's admission webhook then records, from the authenticated request:

- `sync.kuvryn.io/approved-by`: the Kubernetes user who approved;
- `sync.kuvryn.io/approved-at`: when;
- `sync.kuvryn.io/approved-digest`: the digest of the plan they approved
  (`status.plan.digest` on the Revision).

These annotations cannot be set or edited by hand: the webhook overwrites them
on every change. Applications discovered from `.ksync.yaml` never carry
approvals from Git. Kuvryn Sync applies only when the approved Revision's current
plan digest still matches; if desired or live state changed before the
rollout started, the Revision returns to AwaitingApproval with an
`ApprovalStale` Event and must be approved again. Once the rollout starts, one
approval covers all of that Revision's hooks and waves, as long as the desired
state stays the one approved; a changed desired state, or a later rollout of
the same Revision (for example self-heal), needs a fresh approval. The applied Revision keeps the record in
`status.approval`, and `ksync history -o json` exports it.

The webhook fails closed: while the controller is unavailable, Applications
cannot be created or updated. With `ENABLE_WEBHOOKS=false` nothing verifies
the approval annotations, anyone who can update an Application can forge
them, and the manager says so at startup; do not disable webhooks where
approvals matter.

## Image automation

An `ImagePolicy` scans a registry and selects the image to run; the
Repository commits that choice back to Git, so every image bump is a normal,
reviewable commit that flows through planning and, if configured, manual
approval.

```yaml
apiVersion: sync.kuvryn.io/v1alpha1
kind: ImagePolicy
metadata:
  name: api
  namespace: payments
spec:
  image: ghcr.io/acme/api
  secretRef:
    name: ghcr-pull # dockerconfigjson, labelled sync.kuvryn.io/registry-credentials: "true"
  interval: 5m
  policy:
    semver:
      range: ">=1.0.0 <2.0.0" # or tagPattern: {regex, order}, or digest: {tag: main}
---
# On the Repository in the same namespace:
spec:
  imageUpdate:
    secretRef:
      name: platform-push # labelled sync.kuvryn.io/git-credentials: "true", allowed to push
    branch: main # defaults to spec.git.revision
    path: apps
```

Mark image references in YAML with Flux-compatible setter comments; Kuvryn Sync
replaces the value with the selected `image:tag@digest`, or only the tag or
name with the `:tag` and `:name` forms:

```yaml
image: ghcr.io/acme/api:1.0.0 # {"$imagepolicy": "payments:api"}
tag: 1.0.0 # {"$imagepolicy": "payments:api:tag"}
```

To scan as soon as CI or the registry publishes an image, add
`spec.webhook.secretRef` to the ImagePolicy (a Secret with `token`) and call
`POST /hooks/imagepolicies/<namespace>/<policy>` on the webhook receiver with
`Authorization: Bearer <token>`, a GitHub package-event signature, or a GitLab
token. Interval scanning continues as a fallback.

Markers may only name ImagePolicies in the Repository's namespace. Kuvryn Sync
commits only when something changed, retries when the branch moved during
the push, and reports the result in the Repository's `ImagesUpdated`
condition (`False` with reason `Disabled` if the manager runs without image
write-back). A policy rescans on its `interval`, when its spec or annotations
change, and on a webhook request; a Repository re-reads its markers only when
a policy's selected image changes. A marker with more than three
colon-separated parts is left alone. Registry requests are rate-limited per registry host and counted in
`kuvryn_sync_image_scans_total`.

## Push webhooks

Instead of waiting for `pollInterval`, Repositories can be fetched as soon as
GitHub or GitLab reports a push. The receiver is off by default. Enable it,
expose its Service through your ingress, and give the Repository a webhook
secret.

- Helm: set `webhookReceiver.enabled=true`. The chart adds
  `--webhook-receiver-bind-address=:9292` to the manager and creates the
  Service `<release>-kuvryn-sync-receiver` on port 80.
- Kustomize (`config/default`): uncomment the two `[RECEIVER]` entries in
  `config/default/kustomization.yaml`. They add the same flag and create the
  Service `kuvryn-sync-receiver` on port 80.

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: platform-webhook
stringData:
  token: <random shared secret>
---
# On the Repository:
spec:
  webhook:
    secretRef:
      name: platform-webhook
```

Point the Git host at `https://<ingress>/hooks/<namespace>/<repository>` with
the same secret: GitHub signs with it (`X-Hub-Signature-256`), GitLab sends it
as `X-Gitlab-Token`; both are checked in constant time. A push whose payload
names the Repository's URL stamps `sync.kuvryn.io/reconcile-requested-at` on the
Repository, which triggers an immediate fetch; other events are ignored.
Unknown Repositories and bad signatures both get 401, so the receiver does not
reveal which Repositories exist; bodies over 1 MB get 413 and unreadable
bodies 400. Each remote address may send a burst of 50 requests, refilled at
five per second, before authentication is checked; behind an ingress every
sender shares the ingress's address. After authentication, each Repository
may send a burst of ten requests, refilled at one request per second.
Requests beyond either limit get 429. All responses are counted in
`kuvryn_sync_webhook_receiver_requests_total`. Polling continues as a fallback.
ImagePolicies with `spec.webhook` are served the same way at
`/hooks/imagepolicies/<namespace>/<name>`, with their own rate limit.

## Sync hooks and waves

Kuvryn Sync applies an Application in groups and waits for each group to be
Healthy before starting the next:

1. **Pre-sync hooks**: objects annotated `sync.kuvryn.io/hook: pre-sync`, such as a
   database migration Job.
2. **Sync waves**, in ascending order of `sync.kuvryn.io/sync-wave` (an integer,
   default `0`, negative allowed). Within a wave, objects apply in kind order
   (Namespaces and CRDs first, then RBAC and config, Services, workloads,
   routes).
3. **Pruning** of objects no longer in desired state.
4. **Post-sync hooks**: objects annotated `sync.kuvryn.io/hook: post-sync`, such as
   a smoke test.

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: migrate
  annotations:
    sync.kuvryn.io/hook: pre-sync
```

Helm's `pre-install`/`pre-upgrade` and `post-install`/`post-upgrade` hooks and
Argo CD's `PreSync`/`PostSync` hooks and `sync-wave` annotations are honoured
the same way, which eases migrations. Objects that must never be applied
during a sync are skipped: `sync.kuvryn.io/hook: skip`, Helm test, delete, and
rollback hooks, and Argo CD `Skip`, `SyncFail`, `PreDelete`, and `PostDelete`
hooks. Argo CD `Sync` objects apply as ordinary objects. An unknown
`sync.kuvryn.io/hook` or Argo CD hook value fails the Revision with
`ValidationFailure` rather than being applied.

A hook that fails (a failed Job, or a `Stalled` resource) fails the Revision
with reason `HookFailed`, naming the hook, and later groups are not applied.
The Revision's `status.hooks` lists each hook with its stage and state. A hook
runs once per rollout: a hook that succeeded is not run again, or reported as
drift, even after `ttlSecondsAfterFinished` deletes it; a hook deleted while
still running fails the Revision; a failed hook runs again when the Revision
is retried. Hook objects are kept after they finish so their logs stay
available, and are deleted and run again when the next Revision syncs. A
rollout paused by a dependency, an approval, or a failure resumes where it
stopped; the Revision's `RolloutComplete` condition turns `True` once every
group is Healthy.

## Helm charts from repositories

Helm Applications can render a chart from a chart repository instead of Git:

```yaml
spec:
  source:
    repositoryRef:
      name: platform # still used for valuesFiles
    path: envs/prod
    render:
      type: helm
      helm:
        chart:
          repository: oci://ghcr.io/acme/charts # or an https:// Helm repository
          name: api
          version: 1.4.2
        valuesFiles: [values.yaml] # relative to path, in Git
        valuesFrom:
          - kind: Secret
            name: api-values
        values:
          replicaCount: 3
```

Values merge in this order, later winning: the chart's defaults,
`valuesFiles`, each `valuesFrom` entry in order, then `values`. Charts are
cached by repository, name, and version, and the pulled archive's sha256 is
recorded on the Revision. `version` must be an exact SemVer version (a leading
`v` is allowed); ranges such as `>=1.0.0` or `1.x` are refused. The cache is
kept per namespace and per credentials, so one namespace never receives a
private chart another namespace pulled. Kuvryn Sync never uses Helm's local repository
configuration, cached credentials, or plugins. Charts pulled from a
repository carry their dependencies; charts rendered from Git must vendor
theirs into `charts/`.

## SOPS-encrypted Secrets

Commit Secrets encrypted with [SOPS](https://getsops.io) using age keys, and
give the Application the private keys:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: sops-age
  labels:
    sync.kuvryn.io/decryption-key: "true"
stringData:
  payments.agekey: AGE-SECRET-KEY-1...
---
# On the Application:
spec:
  decryption:
    provider: sops
    secretRef:
      name: sops-age
```

Kuvryn Sync decrypts each SOPS file in memory as the `yaml` and `kustomize`
renderers read it, and Helm `valuesFiles` as the `helm` renderer reads them, before Kustomize transforms anything, and verifies the
SOPS MAC. Plaintext is never written to disk, plans, status, or Events. Every
entry ending in `.agekey` is tried; only age keys are supported. The key
Secret must carry the label, like Git credential Secrets. An encrypted file
in an Application without `spec.decryption` fails the Revision instead of
being applied as ciphertext. Only files a render actually reads are decrypted,
so another team's encrypted files elsewhere in a shared repository do not
affect this Application.

## Notifications

Applications can send lifecycle notifications to a `NotificationSink` in their
namespace. A sink reads its destination from a Secret: `url` (https only) and,
for `type: webhook`, `hmacKey`.

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: audit-webhook
stringData:
  url: https://audit.example.com/kuvryn-sync
  hmacKey: <random shared key>
---
apiVersion: sync.kuvryn.io/v1alpha1
kind: NotificationSink
metadata:
  name: audit
spec:
  type: webhook # or slack, for a Slack incoming webhook URL
  secretRef:
    name: audit-webhook
---
# On the Application:
spec:
  notifications:
    - sinkRef:
        name: audit
      events: [AwaitingApproval, Healthy, Failed, RolledBack]
```

Each event fires once per transition, not on every reconcile. Webhook bodies
are JSON with the Application, Revision, source revision, redacted message,
plan summary, and, for `AwaitingApproval`, the `ksync approve` command; the
`X-Kuvryn-Sync-Signature` header is `sha256=` plus the hex HMAC-SHA256 of the body
with `hmacKey`. Slack sinks receive a short text message.

Delivery is best effort and never blocks reconciliation: notifications wait
in a bounded in-memory queue, get up to three delivery attempts with backoff,
and are lost if the controller restarts. Failed deliveries emit a
`NotificationFailed` Warning Event and count in
`kuvryn_sync_notification_deliveries_total{result="failed"}`. A missing sink or
invalid Secret sets the Application condition `NotificationsReady=False`.
Sinks are called from the controller's network, so restrict who can create
NotificationSinks if internal endpoints must not be reachable. Redirects are
not followed (a 3xx counts as a failed delivery), and delivery errors never
include the sink URL, which for Slack is itself a credential.

## Safety defaults

- Server-Side Apply conflicts fail by default.
- Secret values are redacted from plans and CLI output.
- Sync state and health state are separate.
- Revision plan/history data is bounded.
- Rollback goes through normal reconcile machinery.

## Health checks

The manager exposes Kubernetes health/readiness probes configured by the
controller-runtime scaffold. Check rollout and logs with the commands below.
They use the Deployment name of the raw manifests; a Helm install names it
`<release>-kuvryn-sync`, such as `kuvryn-sync-kuvryn-sync` for the release `kuvryn-sync`.

```sh
kubectl -n kuvryn-sync-system rollout status deployment/kuvryn-sync-controller-manager
kubectl -n kuvryn-sync-system logs deployment/kuvryn-sync-controller-manager -c manager
```

## Drift detection for other kinds

Kuvryn Sync watches a managed kind for drift only if its controller service account
may `list` and `watch` it; it checks this with a SelfSubjectAccessReview when it
first applies the kind and again every resync interval. To get immediate drift
detection for, say, cert-manager Certificates, grant the controller read access
to them:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: kuvryn-sync-watch-certificates
rules:
  - apiGroups: ["cert-manager.io"]
    resources: ["certificates"]
    verbs: ["list", "watch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: kuvryn-sync-watch-certificates
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: kuvryn-sync-watch-certificates
subjects:
  # Raw manifests. For a Helm install, use <release>-kuvryn-sync instead, such as
  # kuvryn-sync-kuvryn-sync for the release kuvryn-sync, in the release's namespace.
  - kind: ServiceAccount
    name: kuvryn-sync-controller-manager
    namespace: kuvryn-sync-system
```

Without the grant, Applications managing that kind are re-checked every
`--drift-resync-interval` (default 5m); `0` disables the resync.

## High availability

Leader election is available through `--leader-elect` and enabled by the chart
values:

```yaml
replicaCount: 2
leaderElection: true
```

Use at least two replicas for controller availability, while remembering that
only the elected leader reconciles at any moment.

## Manager flags and environment

| Flag                                         | Default   | Meaning                                                                                            |
| -------------------------------------------- | --------- | -------------------------------------------------------------------------------------------------- |
| `--default-service-account`                  | empty     | Service account for Applications that set none; empty refuses them. Helm `defaultServiceAccount`.  |
| `--drift-resync-interval`                    | `5m`      | Drift re-check for Applications with unwatched kinds; `0` disables it. Helm `driftResyncInterval`. |
| `--webhook-receiver-bind-address`            | empty     | Push webhook receiver address, such as `:9292`; empty disables it. Helm `webhookReceiver.enabled`. |
| `--leader-elect`                             | `false`   | Leader election; the chart enables it. Helm `leaderElection`.                                      |
| `--metrics-bind-address`                     | `0`       | Metrics address, such as `:8443`; `0` disables metrics. The chart and raw manifests use `:8443`.   |
| `--metrics-secure`                           | `true`    | Serve metrics over HTTPS with authentication and authorization.                                    |
| `--health-probe-bind-address`                | `:8081`   | `/healthz` and `/readyz` address.                                                                  |
| `--webhook-cert-path`, `--metrics-cert-path` | empty     | Directories holding the webhook and metrics certificates.                                          |
| `--webhook-cert-name`, `--metrics-cert-name` | `tls.crt` | Certificate file name in those directories.                                                        |
| `--webhook-cert-key`, `--metrics-cert-key`   | `tls.key` | Key file name in those directories.                                                                |
| `--enable-http2`                             | `false`   | Enable HTTP/2 for the metrics and webhook servers.                                                 |
| `--zap-log-level`, `--zap-devel`             |           | controller-runtime logging options.                                                                |

| Environment variable                                                 | Meaning                                                                                   |
| -------------------------------------------------------------------- | ----------------------------------------------------------------------------------------- |
| `ENABLE_WEBHOOKS=false`                                              | Disables the admission webhooks; see [Manual approval](#manual-approval) before using it. |
| `OTEL_EXPORTER_OTLP_ENDPOINT`, `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT`  | Turn on trace export; see [Metrics and tracing](#metrics-and-tracing).                    |
| `OTEL_SERVICE_NAME`, `OTEL_RESOURCE_ATTRIBUTES`, `OTEL_SDK_DISABLED` | Name the service (default `kuvryn-sync`), add resource attributes, or turn tracing off.   |

## Source cache

Each replica keeps a bare clone of every Repository and a checkout of every
commit it renders, under `/tmp/kuvryn-sync-source-cache` (an `emptyDir` in the
chart). Every hour it removes checkouts that no Revision or Repository still
refers to, and clones of Repositories that no longer exist, once they have gone
unused for an hour. Checkouts for Revisions kept by history retention stay, so
the cache grows with `spec.history` and the number of Repositories, not with
every commit ever rendered. Size the volume for that. Pulled Helm charts are
cached beside the clones and removed once no render has used them for a day;
every render of a Helm Application uses its chart.

Pruning removes checkouts and whole clones, but a Repository's bare clone keeps
every object it has fetched: Kuvryn Sync does not garbage-collect Git objects. Size
the volume for each Repository's full history and fetch churn, not only the
retained checkouts; a clone shrinks back only when its Repository is deleted or
the Pod restarts with an empty cache.

The chart's `emptyDir` has no size limit and counts against the node's
ephemeral storage. On tight nodes, add an `ephemeral-storage` request and
limit to the chart's `resources` value so the scheduler accounts for it; a
Pod that exceeds its limit is evicted and starts with an empty cache, which
Kuvryn Sync refills on the next fetch. Each prune is logged as
`Pruned Git source cache` or `Pruned Helm chart cache` with the number of
entries removed.

## Metrics and tracing

Kuvryn Sync registers Prometheus collectors with bounded labels. Expose metrics
using the generated service and your cluster's monitoring stack.

| Metric                                               | Type      | Labels                                              |
| ---------------------------------------------------- | --------- | --------------------------------------------------- |
| `kuvryn_sync_application_reconcile_total`            | counter   | `namespace`, `sync`, `health`, `phase`, `result`    |
| `kuvryn_sync_application_reconcile_duration_seconds` | histogram | `namespace`, `sync`, `health`, `phase`, `result`    |
| `kuvryn_sync_lifecycle_events_total`                 | counter   | `namespace`, `sync`, `health`, `phase`, `reason`    |
| `kuvryn_sync_notification_deliveries_total`          | counter   | `type`, `result` (`delivered`, `failed`, `dropped`) |
| `kuvryn_sync_webhook_receiver_requests_total`        | counter   | `result`                                            |
| `kuvryn_sync_image_scans_total`                      | counter   | `result` (`success`, `error`)                       |

`result` on the reconcile metrics is `success` or `error`; `reason` is the
Event reason, such as `Diagnosed` or `HealthFailure`. The receiver's `result`
is `accepted`, `ignored`, `ping`, `unauthorized`, `rate_limited`,
`too_large`, `bad_request`, `repository_mismatch`, or `error`. The
controller-runtime and Go runtime metrics are exported too.

Tracing is off by default. Set `OTEL_EXPORTER_OTLP_ENDPOINT` (or
`OTEL_EXPORTER_OTLP_TRACES_ENDPOINT`) on the manager and Kuvryn Sync exports an
`Application/Reconcile` span for every Application reconcile over OTLP gRPC;
HTTP/protobuf export is not supported. The OTLP gRPC exporter's variables apply,
such as `OTEL_EXPORTER_OTLP_HEADERS` and `OTEL_EXPORTER_OTLP_INSECURE`, as do
`OTEL_SERVICE_NAME` (default `kuvryn-sync`) and `OTEL_RESOURCE_ATTRIBUTES`;
`OTEL_SDK_DISABLED=true` turns tracing off again. Export failures are logged by
the manager. With Helm, pass the variables through `extraEnv`:

```yaml
extraEnv:
  - name: OTEL_EXPORTER_OTLP_ENDPOINT
    value: http://otel-collector.observability:4317
```

A span records the reconcile's error, redacted like status messages, and does
not name the Application.

## Repository-driven Application discovery

Repositories can bootstrap Applications from `.ksync.yaml` files in Git. Leave
`spec.applicationConfigPaths` empty to read the root `.ksync.yaml`, or list one
or more repository-relative `.ksync.yaml` paths for monorepos.

Operational rules:

- every configured path must be relative, unique, inside the repository, and
  named `.ksync.yaml`;
- each file can contain one `Application` or an `applications:` list;
- Application names must be unique across all configured files;
- discovered Applications carry `sync.kuvryn.io/repository` and
  `sync.kuvryn.io/discovered-from` metadata;
- removing a discovered Application from Git prunes the managed Application CR.

## Events and Conditions

Use Kubernetes-native surfaces first:

```sh
kubectl describe app <name> -n <namespace>
kubectl get events -n <namespace> --sort-by=.lastTimestamp
kubectl get revisions.sync.kuvryn.io -n <namespace>
```

Kuvryn Sync emits lifecycle Events and writes Conditions for readiness, failure, and
rollout states. A `Diagnosed` Warning Event names the first root cause each
time the set of causes in `status.diagnosis` changes. A `PruneSkipped` Warning
Event names the managed resources prune kept, up to five, once per attempt at a
Revision that leaves them out of desired state; see [Apply and prune](concepts.md#apply-and-prune).

## Diagnosis permissions

To diagnose an unhealthy Application, Kuvryn Sync reads, as the Application's
service account, in the destination namespace:

- `list` on `replicasets` (apps), `pods` and `endpointslices`
  (discovery.k8s.io);
- `get` on the `configmaps`, `secrets`, `serviceaccounts`,
  `persistentvolumeclaims` and `services` its resources refer to;
- `get` on the target of each HorizontalPodAutoscaler's `scaleTargetRef`,
  which can be any kind;
- `get` on the `persistentvolumes` its claims are bound to. PersistentVolumes
  are cluster-scoped, so only a ClusterRole bound with a ClusterRoleBinding
  grants this; without it, the edge from a claim to its volume is simply
  `unreadable`.

Secrets, ConfigMaps and ServiceAccounts are read as metadata only; their data
never leaves the API server, although RBAC still asks for the `get` verb. A
read the account may not make is not an error: the diagnosis only goes less
deep, a reference it could not check is never blamed as missing, and a list it
could not make is named in the message of a cause that found no deeper
evidence. Reads are bounded: at most 20 label selectors and 20 Services' EndpointSlices,
100 objects per list, 100 referenced objects and 500 objects in all.

## History retention

Set `spec.history.limit` on Applications to bound Revision count. Retention is
per Application and prevents unbounded status/API growth.

## Release validation

Release validation should use a repeatable CI or cluster environment that matches
the target architecture. Published images are built by GitHub Actions and pushed
to GHCR.
