# kuvryn-sync-rebrand

Status: complete

## Problem

Solder is joining the Kuvryn product family, the Azrty products built on one
design system and one naming scheme, as **Kuvryn Sync**, and it is about to
gain a web console designed under that name. Today every surface says
"solder":

- the `solder.io` API group and its labels and annotations;
- the `solder` CLI;
- the Go module and repository;
- the container image, the Helm chart and the docs.
  A console branded Kuvryn Sync on top of a controller branded Solder would
  confuse operators and split the documentation. The maintainer decided on a
  full rename with a clean break, so the change must be made once, completely,
  before the console ships and before any more users adopt the old names.

## Users

- **Platform operators** install and upgrade the controller. They need one
  consistent name in the chart, image, namespace and docs, and a clear
  statement that 0.3.x solder installs are not migrated.
- **Application teams** write Application, Repository and related manifests
  and `.solder.yaml` discovery files. They need the new API group and
  annotation keys documented, with examples.
- **CLI users** approve, sync, roll back and diagnose. They need the `ksync`
  binary, with the same subcommands and flags.
- **Contributors** build and test. They need the module path, the Makefile,
  CI, e2e fixtures and the docs to agree on the new names.

## In scope

- [S-1] The API group moves from solder.io to `sync.kuvryn.io` for every
  CRD: Repository, Application, Revision, HealthCheck, NotificationSink and
  ImagePolicy. Markers, generated CRDs, RBAC, webhooks, the kubebuilder
  PROJECT file (domain kuvryn.io, group `sync`) and every code reference
  follow.
- [S-2] Every label, annotation and finalizer key under the `solder.io/`
  prefix moves to `sync.kuvryn.io/`, with the same suffixes: application,
  approved-revision, rollback-*, git-credentials, registry-credentials,
  decryption-key, hook, sync-wave, prune and the rest. The code, tests and
  docs follow.
- [S-3] The Go module moves from `github.com/azrtydxb/solder` to
  `github.com/azrtydxb/kuvryn-sync`. The kubebuilder `projectName` and
  `repo` follow.
- [S-4] The CLI binary and command name become `ksync`, including usage
  text, help and `ksync version`. The manager is the same binary; `ksync`
  with no subcommand starts the controller.
- [S-5] The container image becomes `ghcr.io/azrtydxb/kuvryn-sync`, and the
  Makefile, Dockerfile, image workflow and installer follow.
- [S-6] The Helm chart is renamed `kuvryn-sync`, with its release-derived
  names, default namespace `kuvryn-sync-system` and the kustomize
  `namePrefix` and namespace.
- [S-7] The Server-Side Apply field manager becomes `kuvryn-sync`, and
  metrics move from `solder_*` to `kuvryn_sync_*`. Event reporting names,
  the OTel service name default and log names follow.
- [S-8] The repository-discovery file `.solder.yaml` is renamed to
  `.ksync.yaml`, and spec.applicationConfigPaths must name `.ksync.yaml`
  files.
- [S-9] All documentation (`README.md`, `docs/*.md`, `CHANGELOG.md`,
  `config/samples/`, `solder-full-spec.md`, which is renamed
  `kuvryn-sync-full-spec.md`) uses Kuvryn Sync, `ksync`, `sync.kuvryn.io` and the new image and
  chart. The product brand line reads "Kuvryn Sync — an Azrty product".
- [S-10] Claude renames the GitHub repository to `azrtydxb/kuvryn-sync` (with
  `gh repo rename`) after the code PR merges. It also renames the e2e fixture
  repository `azrtydxb/solder-e2e-app` to `azrtydxb/kuvryn-sync-e2e-app` and
  pushes its discovery file as `.ksync.yaml`. CI and e2e pass under the new
  names.
- [S-11] The first Kuvryn Sync release, v0.4.0, is cut. It continues the
  version line and changelog from solder v0.3.0, and its release notes state
  the clean break. Claude makes the new GHCR package public through the
  GitHub web UI, since GitHub has no API for it, and verifies an anonymous
  pull. docs/upgrade.md says that solder.io installs are not
  migrated and how to reinstall.

## Out of scope

- Any migration tooling or conversion from `solder.io` objects or labels.
  This is a clean break: existing installs reinstall and re-create their
  objects, and may take workloads over with `conflictPolicy: adopt`.
- Keeping `solder` compatibility aliases: no `solder` CLI alias, no
  `solder.io` annotation fallback, no old image tags republished.
- Touching the live `solder-system` install on the kw cluster (v0.1.11).
- The web console itself, which has its own spec: kuvryn-sync-console.
- New features or behaviour changes. This is a rename only.

## Constraints

- Behaviour must be identical after the rename: the full unit, envtest, lint
  and Kind e2e suites pass with only name changes.
- No `solder` or solder.io string may remain in code, config, charts or
  user docs. The only exceptions are historical CHANGELOG entries and the
  clean-break note. A grep-based test enforces this.
- The kubebuilder PROJECT file has to be edited by hand for the domain and
  repo, because no kubebuilder command renames them. This is the one allowed
  exception to the "never edit PROJECT" rule. `make manifests generate` must
  then leave no diff.
- CI keeps running on the lab ARC runners and publishes via the kw publish
  runners.

## Interfaces

| Before                                             | After                                                |
| -------------------------------------------------- | ---------------------------------------------------- |
| API group solder.io, e.g. `applications.solder.io` | `sync.kuvryn.io`, e.g. `applications.sync.kuvryn.io` |
| `apiVersion: solder.io/v1alpha1`                   | `apiVersion: sync.kuvryn.io/v1alpha1`                |
| Labels and annotations `solder.io/<key>`           | `sync.kuvryn.io/<key>`                               |
| CLI `solder <cmd>`                                 | `ksync <cmd>`                                        |
| Go module `github.com/azrtydxb/solder`             | `github.com/azrtydxb/kuvryn-sync`                    |
| Image `ghcr.io/azrtydxb/solder`                    | `ghcr.io/azrtydxb/kuvryn-sync`                       |
| Helm chart `solder`                                | `kuvryn-sync`                                        |
| Namespace `solder-system`                          | `kuvryn-sync-system`                                 |
| SSA field manager `solder`                         | `kuvryn-sync`                                        |
| Metrics `solder_*`                                 | `kuvryn_sync_*`                                      |
| Discovery file `.solder.yaml`                      | `.ksync.yaml`                                        |

## Data

- No stored data moves: the change is a clean break.
- The new CRDs are new, empty resources. Objects in the old `solder.io` group
  on a cluster are untouched and ignored by the new controller.
- Workloads that a solder install applied keep their `solder.io/*` labels and
  `solder` field ownership until an operator re-creates their Applications
  under `sync.kuvryn.io`. The docs describe adopting them with
  `conflictPolicy: adopt`.

## Edge cases

- Both products installed in one cluster: the API groups, namespaces, field
  managers and labels differ, so each ignores the other's objects. The docs
  warn not to point both at the same workloads.
- A workload previously applied by solder, adopted by a new Application: its
  fields are owned by manager `solder`. With the default `conflictPolicy:
fail`, the plan reports conflicts, and adopt takes ownership. This is
  documented, and e2e covers adopt already.
- String concatenation that builds keys, such as `"solder.io/" + x`, and
  prefix constants must all move. The grep test catches literal leftovers.
- The webhook paths generated from the group, and the configuration names
  (`kuvryn-sync-validating-webhook-configuration` and so on), must match
  between kustomize and the chart. The existing chart-and-role pinning tests
  must pass.
- The GitHub repository rename redirects old clone URLs, but GHCR does not
  redirect images. The chart and docs must reference only the new image.

## Failure modes

- GHCR: the new package `ghcr.io/azrtydxb/kuvryn-sync` is created private by
  default, just as solder's was. The release is not usable until the package
  is made public, a manual web-UI step. The release checklist includes it,
  and an anonymous-pull check verifies it.
- GitHub rename: until the repository is renamed, the module path
  `github.com/azrtydxb/kuvryn-sync` does not resolve for `go install`. Rename
  the repository before tagging the release.
- The e2e fixture repository is referenced by URL in the tests. If it isn't
  updated or renamed together with the code, e2e fails, and CI catches it.

## Acceptance criteria

- [ ] [S-1] `TestCRDsUseTheKuvrynSyncGroup` reads `config/crd/bases` and fails
      if any CRD is not in group `sync.kuvryn.io`, or if any of the six kinds is
      missing.
- [ ] [S-1] [S-3] `make manifests generate` leaves no diff, and `go build ./...`
      succeeds with module `github.com/azrtydxb/kuvryn-sync`. `TestNoSolderNameRemains`
      fails if any Go import still names `github.com/azrtydxb/solder`.
- [ ] [S-2] `TestE2E` (`make test-e2e`) fails if the applied objects lack
      `sync.kuvryn.io/application`, or if approving with
      `sync.kuvryn.io/approved-revision` does not deploy.
- [ ] [S-4] `TestKsyncVersionAndHelp` fails if `ksync version` does not print
      `ksync <version>`, or if `ksync help` names a command other than as
      `ksync <cmd>`.
- [ ] [S-5] [S-6] `TestHelmChartUsesKuvrynSyncNames` renders
      `charts/kuvryn-sync` with `helm template`. It fails if the image is not
      `ghcr.io/azrtydxb/kuvryn-sync:v<appVersion>` or a resource name still
      says solder. `helm lint charts/kuvryn-sync` passes.
- [ ] [S-7] `TestApplyUsesTheKuvrynSyncFieldManager` fails if the applier's
      field manager is not `kuvryn-sync`, and `TestMetricNamesUseKuvrynSyncPrefix`
      fails if a registered metric does not start with `kuvryn_sync_`.
- [ ] [S-8] `TestDiscoveryReadsKsyncYaml` fails if a repository holding
      `.ksync.yaml` discovers nothing, or if one holding only `.solder.yaml`
      discovers anything.
- [ ] [S-9] `TestNoSolderNameRemains` scans the repository. It fails if
      `solder` appears outside CHANGELOG history, the clean-break note in
      `docs/upgrade.md`, `.procoder/` history, and the history section of
      `kuvryn-sync-full-spec.md`.
- [ ] [S-10] `gh repo view azrtydxb/kuvryn-sync` succeeds, and CI (Tests, Lint,
      E2E, Image) is green on it. `TestE2E` (`make test-e2e`) fails if the
      fixture URL `azrtydxb/kuvryn-sync-e2e-app` is wrong, because it cannot
      clone the fixture. `.ksync.yaml` discovery is covered by
      `TestDiscoveryReadsKsyncYaml`, since the e2e suite does not use discovery.
- [ ] [S-11] `TestReleaseImageIsPublic` sends an anonymous `curl` request for
      `ghcr.io/v2/azrtydxb/kuvryn-sync/manifests/v0.4.0`, and fails if the
      package is still private (401 or 403). `gh release view v0.4.0` shows
      the release, and `docs/upgrade.md` states that solder.io installs are not
      migrated.

## Open questions

<!-- All resolved with the maintainer on 2026-09-25; decisions recorded above. -->
