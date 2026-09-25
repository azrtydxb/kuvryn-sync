# kuvryn-sync-rebrand — implementation plan

Status: draft
Spec: .procoder/specs/kuvryn-sync-rebrand.md

## Goal

Rename every Solder identity to Kuvryn Sync (API group `sync.kuvryn.io`, CLI
`ksync`, `kuvryn-sync` everywhere else) with no behaviour change, then rename
the repositories and ship v0.4.0 publicly.

## Architecture

The rename happens in one branch as a sequence of commits, one identity per
task, each pinned by a test that fails while the old name remains. Generated
files (CRDs, RBAC, webhooks, deepcopy) are regenerated with
`make manifests generate`, never hand-edited. The kubebuilder `PROJECT` file
is the single documented exception. A final repo-wide test forbids any
remaining `solder` string outside history. The GitHub repository renames,
the GHCR visibility change and the release come after the code merges.

## Constraints

- Behaviour must not change. `make test`, `make lint`, and the Kind e2e must
  pass after every task.
- The names are exact:
  - API group `sync.kuvryn.io`, and label/annotation prefix `sync.kuvryn.io/`;
  - CLI binary and command name `ksync`;
  - Go module `github.com/azrtydxb/kuvryn-sync`;
  - image `ghcr.io/azrtydxb/kuvryn-sync`, and Helm chart directory and name
    `kuvryn-sync`;
  - namespace `kuvryn-sync-system`, and kustomize `namePrefix: kuvryn-sync-`;
  - SSA field manager `kuvryn-sync`, and metric prefix `kuvryn_sync_`;
  - discovery file `.ksync.yaml`;
  - first release `v0.4.0`;
  - fixture repository `azrtydxb/kuvryn-sync-e2e-app`.
- No migration code and no `solder` compatibility aliases. This is a clean
  break.
- History is kept only in:
  - CHANGELOG entries for 0.3.0 and earlier;
  - the clean-break note in `docs/upgrade.md`;
  - `.procoder/`;
  - the "History" section of `kuvryn-sync-full-spec.md`.
- Never hand-edit `config/crd/bases`, `config/rbac/role.yaml`,
  `config/webhook/manifests.yaml` or `zz_generated.*`. Regenerate them.
- Commits use imperative subjects of 72 characters or fewer, with a why-body
  and no attribution.

## Task 1: Move the Go module to github.com/azrtydxb/kuvryn-sync

Files:

- `go.mod`: the module line.
- Every `*.go` file that imports `github.com/azrtydxb/solder/...`.
- `PROJECT`: `repo`, the `path` of every resource, and `projectName`.
- `internal/brand/names_test.go`: new; the repo-wide name guard.

Interfaces: produces module path `github.com/azrtydxb/kuvryn-sync` and test
`TestNoSolderNameRemains` in package `brand`, which later tasks extend.

- [ ] Write the failing guard `internal/brand/names_test.go`:
  ```go
  package brand

  import (
  	"os"
  	"os/exec"
  	"strings"
  	"testing"
  )

  // TestNoSolderNameRemains fails while an old product name is left in the
  // listed scopes. Later tasks widen scopes until it covers the repository.
  func TestNoSolderNameRemains(t *testing.T) {
  	root := "../.."
  	out, err := exec.Command("git", "-C", root, "grep", "-n", "github.com/azrtydxb/solder", "--", "*.go", "go.mod", "PROJECT").CombinedOutput()
  	if err == nil {
  		t.Fatalf("old module path remains:\n%s", out)
  	}
  	if len(strings.TrimSpace(string(out))) != 0 && !strings.Contains(string(out), "exit status 1") {
  		t.Fatalf("git grep failed: %s", out)
  	}
  	_ = os.Getenv
  }
  ```
  Run `go test ./internal/brand/`, and expect it to FAIL with "old module
  path remains".
- [ ] Change the module line in `go.mod` to `module github.com/azrtydxb/kuvryn-sync`.
      Rewrite imports with
      `git grep -l 'github.com/azrtydxb/solder' -- '*.go' | xargs sed -i '' 's#github.com/azrtydxb/solder#github.com/azrtydxb/kuvryn-sync#g'`.
      In `PROJECT`, set `projectName: kuvryn-sync`,
      `repo: github.com/azrtydxb/kuvryn-sync`, and every resource `path:` to
      `github.com/azrtydxb/kuvryn-sync/api/v1alpha1`.
- [ ] Run `go build ./... && go test ./internal/brand/`, and expect PASS.
      Run `make test`, and expect PASS.
- [ ] Commit "Move the Go module to github.com/azrtydxb/kuvryn-sync".

## Task 2: Move every CRD to the sync.kuvryn.io API group

Files:

- `api/v1alpha1/groupversion_info.go`: the `+groupName` marker and
  `SchemeGroupVersion`.
- Every `+kubebuilder:rbac:groups=solder.io` marker in `internal/controller/*.go`.
- The webhook `+kubebuilder:webhook` markers in `internal/webhook/v1alpha1/*.go`:
  `groups=sync.kuvryn.io`, with paths regenerated.
- `PROJECT`: `domain: kuvryn.io` and `group: sync` for every resource.
- `config/crd/bases/*`, `config/rbac/role.yaml`, `config/webhook/manifests.yaml`:
  regenerated.
- `config/crd/kustomization.yaml`: resource file names become
  `sync.kuvryn.io_*.yaml`.
- `charts/solder/templates/*.yaml`: RBAC and webhook rules use
  `sync.kuvryn.io`.
- Tests that assert the group string.
- `internal/brand/crd_test.go`: new.

Interfaces: produces group `sync.kuvryn.io`, CRD names `<plural>.sync.kuvryn.io`
and the CRD file names `config/crd/bases/sync.kuvryn.io_<plural>.yaml`.

- [ ] Write the failing test `internal/brand/crd_test.go`:
  ```go
  package brand

  import (
  	"os"
  	"path/filepath"
  	"strings"
  	"testing"
  )

  func TestCRDsUseTheKuvrynSyncGroup(t *testing.T) {
  	want := []string{"applications", "repositories", "revisions", "healthchecks", "notificationsinks", "imagepolicies"}
  	for _, plural := range want {
  		path := filepath.Join("..", "..", "config", "crd", "bases", "sync.kuvryn.io_"+plural+".yaml")
  		data, err := os.ReadFile(path)
  		if err != nil {
  			t.Fatalf("missing CRD %s: %v", path, err)
  		}
  		if !strings.Contains(string(data), "group: sync.kuvryn.io") {
  			t.Errorf("%s is not in group sync.kuvryn.io", path)
  		}
  	}
  	matches, _ := filepath.Glob(filepath.Join("..", "..", "config", "crd", "bases", "solder.io_*.yaml"))
  	if len(matches) > 0 {
  		t.Errorf("old CRDs remain: %v", matches)
  	}
  }
  ```
  Run `go test ./internal/brand/ -run TestCRDsUseTheKuvrynSyncGroup`, and
  expect it to FAIL with "missing CRD".
- [ ] Set `// +groupName=sync.kuvryn.io` and
      `Group: "sync.kuvryn.io"` in `groupversion_info.go`.
      Replace `groups=solder.io` with `groups=sync.kuvryn.io` in every rbac and
      webhook marker. In `PROJECT`, set `domain: kuvryn.io` at the top and on
      each resource, and `group: sync`.
- [ ] Run `rm config/crd/bases/solder.io_*.yaml && make manifests generate`.
      Update `config/crd/kustomization.yaml` to the new file names. In
      `charts/solder/templates`, replace `solder.io` API groups with
      `sync.kuvryn.io`. Webhook `path`s become
      `/mutate-sync-kuvryn-io-v1alpha1-application` and
      `/validate-sync-kuvryn-io-v1alpha1-healthcheck`, exactly as
      controller-gen writes them into `config/webhook/manifests.yaml`.
- [ ] Run `go test ./internal/brand/ && make test`, and expect PASS.
      `TestHelmChartRoleMatchesGeneratedRole` must pass.
- [ ] Commit "Move every CRD to the sync.kuvryn.io API group".

## Task 3: Move labels, annotations, and finalizers to the sync.kuvryn.io/ prefix

Files:

- `api/v1alpha1/*_types.go`: the annotation constants.
- `internal/applier/*.go`: the label keys.
- `internal/controller/*.go`: finalizer, credential labels and discovery
  annotations.
- `internal/ordering/ordering.go`: hook and sync-wave keys.
- `internal/prune/prune.go`: the opt-out key.
- `internal/webhook/v1alpha1/*.go`.
- Every test that literally uses `solder.io/`.
- `internal/brand/names_test.go`: extended.

Interfaces: produces constants with unchanged Go names and new values, for
example `applier.ApplicationLabelKey = "sync.kuvryn.io/application"`,
`corev1alpha1.ApprovedRevisionAnnotation = "sync.kuvryn.io/approved-revision"`,
and `ordering` keys `sync.kuvryn.io/hook` and `sync.kuvryn.io/sync-wave`.

- [ ] Extend `TestNoSolderNameRemains` with a second grep, for the pattern
      `solder\.io/` over `*.go`, `config/**` and `charts/**`, excluding
      `config/crd/bases`. Run it, and expect it to FAIL with "old key prefix
      remains".
- [ ] Rewrite the prefix:
      `git grep -l 'solder\.io/' -- '*.go' 'config' 'charts' | xargs sed -i '' 's#solder\.io/#sync.kuvryn.io/#g'`.
      Then grep for keys built by concatenation, such as `"solder.io"+`, and fix
      them by hand.
- [ ] Run `make manifests generate && make test`, and expect PASS. Run
      `go test ./internal/brand/`, and expect PASS.
- [ ] Commit "Move labels, annotations, and finalizers to sync.kuvryn.io/".

## Task 4: Rename the field manager, metrics, and runtime names

Files:

- `internal/applier/applier.go`: `FieldManager = "kuvryn-sync"`.
- `internal/ops/metrics.go` and `internal/imagepolicy/imagepolicy.go`:
  metric names with the `kuvryn_sync_` prefix.
- `internal/receiver/receiver.go`, `internal/notify/notify.go` and every
  other `prometheus` registration: grep `Name: "solder_`.
- `internal/ops/tracing.go`: default service name `kuvryn-sync`, and tracer
  names `github.com/azrtydxb/kuvryn-sync/...`.
- `internal/controller/repository_controller.go`:
  `defaultSourceCacheDir = "kuvryn-sync-source-cache"`.
- `cmd/main.go`: leader-election ID `kuvryn-sync.kuvryn.io`, event recorder
  names and logger names.
- `api/v1alpha1/repository_types.go`: the Git author default
  `kuvryn-sync@localhost`.
- `internal/applier/applier_test.go` and `internal/ops/metrics_test.go`: new
  or extended.

Interfaces: produces `applier.FieldManager == "kuvryn-sync"` and metric
names `kuvryn_sync_application_reconcile_total`,
`kuvryn_sync_application_reconcile_duration_seconds`,
`kuvryn_sync_lifecycle_events_total`, `kuvryn_sync_image_scans_total`,
`kuvryn_sync_webhook_receiver_requests_total` and
`kuvryn_sync_notification_deliveries_total`.

- [ ] Write the failing tests:
  ```go
  func TestApplyUsesTheKuvrynSyncFieldManager(t *testing.T) {
  	if FieldManager != "kuvryn-sync" {
  		t.Fatalf("FieldManager = %q, want kuvryn-sync", FieldManager)
  	}
  }
  ```
  in `internal/applier/applier_test.go`, and in `internal/ops/metrics_test.go`:
  ```go
  func TestMetricNamesUseKuvrynSyncPrefix(t *testing.T) {
  	families, err := metrics.Registry.Gather()
  	if err != nil {
  		t.Fatal(err)
  	}
  	for _, f := range families {
  		if strings.HasPrefix(f.GetName(), "solder_") {
  			t.Errorf("metric %s keeps the old prefix", f.GetName())
  		}
  	}
  }
  ```
  Run `go test ./internal/applier/ ./internal/ops/`, and expect it to FAIL
  with "want kuvryn-sync" and "keeps the old prefix".
- [ ] Rename the values listed under Files. Run
      `git grep -n 'Name: *"solder_'` and expect no output.
- [ ] Run `make test`, and expect PASS.
- [ ] Commit "Rename the field manager, metrics, and runtime names".

## Task 5: Rename the CLI to ksync

Files:

- `internal/cli/*.go`: every `newFlagSet("solder …")`, usage and help text,
  `version` output `ksync <version>`, and the approve command hint.
- `internal/controller/application_controller.go`: `ApproveCommand` in
  notifications becomes `ksync approve …`.
- `Makefile`: the build output becomes `bin/ksync`, and the `run` target
  follows.
- `Dockerfile`: the build output is `ksync`, copied to `/ksync`, with
  `ENTRYPOINT ["/ksync"]`.
- `internal/cli/version_test.go`: extended.
- `internal/cli/help_test.go`: new.

Interfaces: produces the binary `ksync` at `/ksync` in the image. `ksync`
with no arguments starts the manager; `ksync <subcommand>` runs the CLI.

- [ ] Write the failing test `internal/cli/help_test.go`:
  ```go
  func TestKsyncVersionAndHelp(t *testing.T) {
  	var out, errb bytes.Buffer
  	if _, code := Run(context.Background(), []string{"version"}, &out, &errb); code != 0 {
  		t.Fatalf("version exit %d: %s", code, errb.String())
  	}
  	if !strings.HasPrefix(out.String(), "ksync ") {
  		t.Fatalf("version = %q, want ksync <version>", out.String())
  	}
  	out.Reset()
  	Run(context.Background(), []string{"help"}, &out, &errb)
  	if strings.Contains(out.String(), "solder") || !strings.Contains(out.String(), "ksync diagnose") {
  		t.Fatalf("help still names the old CLI:\n%s", out.String())
  	}
  }
  ```
  Run `go test ./internal/cli/ -run TestKsyncVersionAndHelp`, and expect it to
  FAIL with "want ksync <version>".
- [ ] Replace `"solder ` with `"ksync ` in `internal/cli`, and in
      `ApproveCommand` in `application_controller.go`. Update the Makefile's
      `go build -o bin/ksync` and the Dockerfile as listed under Files.
- [ ] Run `go test ./internal/cli/ && make test`, and expect PASS.
- [ ] Commit "Rename the CLI to ksync".

## Task 6: Read .ksync.yaml for repository discovery

Files:

- `internal/controller/repository_controller.go`:
  `solderConfigFileName` becomes `configFileName = ".ksync.yaml"`.
- `internal/controller/repository_discovery.go`: the error messages, and
  the check that config paths must name `.ksync.yaml` files.
- `internal/controller/*_test.go`: fixtures that write `.solder.yaml`.
- `internal/controller/discovery_file_test.go`: new.

Interfaces: produces the constant `configFileName = ".ksync.yaml"`, used by
`solderConfigPaths`, which is renamed `configPaths`, and by
`applicationsFromSolderFile`, which is renamed `applicationsFromConfigFile`.

- [ ] Write the failing test `internal/controller/discovery_file_test.go`,
      using the existing envtest pattern:
  ```go
  func TestDiscoveryReadsKsyncYaml(t *testing.T) {
  	dir := t.TempDir()
  	app := "applications:\n  - name: web\n    source:\n      path: web\n"
  	if err := os.WriteFile(filepath.Join(dir, ".ksync.yaml"), []byte(app), 0o600); err != nil {
  		t.Fatal(err)
  	}
  	repo := &corev1alpha1.Repository{ObjectMeta: metav1.ObjectMeta{Name: "r", Namespace: "default"}}
  	apps, found, err := applicationsFromConfigFile(repo, dir, ".ksync.yaml")
  	if err != nil || !found || len(apps) != 1 {
  		t.Fatalf("apps=%v found=%v err=%v, want one Application from .ksync.yaml", apps, found, err)
  	}
  	old := t.TempDir()
  	if err := os.WriteFile(filepath.Join(old, ".solder.yaml"), []byte(app), 0o600); err != nil {
  		t.Fatal(err)
  	}
  	paths, _ := configPaths(repo)
  	for _, p := range paths {
  		if _, found, _ := applicationsFromConfigFile(repo, old, p); found {
  			t.Fatalf("a repository with only .solder.yaml discovered %s", p)
  		}
  	}
  }
  ```
  Run it, and expect it to FAIL to compile with
  "undefined: applicationsFromConfigFile".
- [ ] Apply the renames listed under Files, and update the fixtures and error
      text to `.ksync.yaml`.
- [ ] Run `go test ./internal/controller/ -run TestDiscoveryReadsKsyncYaml && make test`,
      and expect PASS.
- [ ] Commit "Read .ksync.yaml for repository discovery".

## Task 7: Rename the image, Helm chart, and kustomize install

Files:

- `charts/solder/` moves to `charts/kuvryn-sync/` (`git mv`):
  - `Chart.yaml`: `name: kuvryn-sync`;
  - `values.yaml`: `image.repository: ghcr.io/azrtydxb/kuvryn-sync`;
  - `templates/_helpers.tpl`: `define "kuvryn-sync.name"` and
    `"kuvryn-sync.fullname"`, with every template using them.
- `config/default/kustomization.yaml`: `namespace: kuvryn-sync-system` and
  `namePrefix: kuvryn-sync-`.
- `config/manager/manager.yaml`, `config/rbac/*`, `config/certmanager/*` and
  `config/network-policy/*`: `app.kubernetes.io/name: kuvryn-sync`.
- `Makefile`: `IMG ?= kuvryn-sync:latest`, the `build-installer` image, and
  every other reference to `ghcr.io/azrtydxb/solder`.
- `.github/workflows/image.yml`: `ghcr.io/${{ github.repository_owner }}/kuvryn-sync`.
  The image name does not follow the repository name, so it is correct
  before the repository is renamed too.
- `internal/controller/rbac_manifest_test.go`: chart paths, and the new
  test.

Interfaces: produces chart path `charts/kuvryn-sync`, the Helm names
`<release>-kuvryn-sync`, namespace `kuvryn-sync-system`, and the image
repository `ghcr.io/azrtydxb/kuvryn-sync`.

- [ ] Write the failing test in `internal/controller/rbac_manifest_test.go`:
  ```go
  func TestHelmChartUsesKuvrynSyncNames(t *testing.T) {
  	out, err := exec.Command("helm", "template", "kuvryn-sync", filepath.Join("..", "..", "charts", "kuvryn-sync")).CombinedOutput()
  	if err != nil {
  		t.Fatalf("helm template: %v\n%s", err, out)
  	}
  	rendered := string(out)
  	if !strings.Contains(rendered, "image: \"ghcr.io/azrtydxb/kuvryn-sync:v") {
  		t.Error("the Deployment does not use ghcr.io/azrtydxb/kuvryn-sync")
  	}
  	if strings.Contains(strings.ToLower(rendered), "solder") {
  		t.Error("the rendered chart still says solder")
  	}
  }
  ```
  Run it, and expect it to FAIL with "helm template" (the chart path does
  not exist).
- [ ] Run `git mv charts/solder charts/kuvryn-sync`, then apply the renames
      listed under Files.
      `TestHelmChartRoleMatchesGeneratedRole` must read from
      `charts/kuvryn-sync/templates/rbac.yaml`.
- [ ] Run `helm lint charts/kuvryn-sync && make test`, and expect PASS.
      Run `bin/kustomize build config/default | grep -c kuvryn-sync-system`, and
      expect a nonzero count.
- [ ] Commit "Rename the image, Helm chart, and kustomize install".

## Task 8: Move the e2e fixture repository and e2e suite to the new names

Files:

- GitHub: `gh repo rename kuvryn-sync-e2e-app -R azrtydxb/solder-e2e-app`,
  and in that repository rename `.solder.yaml` (and any nested copies) to
  `.ksync.yaml`, update the manifests' `apiVersion` and annotations to
  `sync.kuvryn.io`, commit, and push to `main`.
- `test/e2e/e2e_test.go`: the fixture URL
  `https://github.com/azrtydxb/kuvryn-sync-e2e-app.git`, namespace
  `kuvryn-sync-system`, deployment names, and the `sync.kuvryn.io` apiVersion
  in inline manifests.
- `test/e2e/e2e_suite_test.go`: the image name `kuvryn-sync:e2e-*`.
- `.github/workflows/test-e2e.yml`: `IMG: kuvryn-sync:e2e-${{ github.sha }}`.
- `hack/migration/*.sh`: the resource names, namespace and field manager
  `kuvryn-sync`.

Interfaces: consumes the fixture repository `azrtydxb/kuvryn-sync-e2e-app` with
`.ksync.yaml` at the root and under its staged path, plus the names from
Tasks 2 to 7.

- [ ] Rename the fixture repository, then run
      `gh api repos/azrtydxb/kuvryn-sync-e2e-app --jq .full_name`. Expect
      `azrtydxb/kuvryn-sync-e2e-app`.
- [ ] In a clone of the fixture repository, rename every `.solder.yaml` to
      `.ksync.yaml`, replace `solder.io` with `sync.kuvryn.io`, then commit and
      push "Rename the fixture to Kuvryn Sync".
- [ ] Update the files listed under Files, then push the branch. CI E2E runs
      on `arc-azrtydxb-amd64`. Expect `Ran 8 of 8 Specs` and `SUCCESS!`. It
      fails if a spec cannot discover the fixture Application.
- [ ] Commit "Move the e2e suite to the Kuvryn Sync names".

## Task 9: Rewrite the documentation and forbid the old name

Files:

- `README.md`, `docs/*.md`, `CONTRIBUTING.md`, `SECURITY.md`, `AGENTS.md`,
  `CLAUDE.md` (if present), `.github/copilot-instructions.md`,
  `skills/procoder/*`, and `config/samples/*.yaml`: every reference uses
  the new names.
- `solder-full-spec.md` moves to `kuvryn-sync-full-spec.md` (`git mv`), with a
  "History" section explaining that the product was named Solder until
  v0.3.0.
- `CHANGELOG.md`: an `## Unreleased` entry, marked **Breaking**, for the
  clean break.
- `docs/upgrade.md`: a section "Moving from Solder 0.3.x to Kuvryn Sync 0.4.0".
  It says no migration is provided, that you reinstall the chart as
  `kuvryn-sync` and re-create objects under `sync.kuvryn.io`, and that you
  can adopt existing workloads with `conflictPolicy: adopt`.
- `internal/brand/names_test.go`: widened to the whole repository.

Interfaces: consumes every name from Tasks 1 to 8. Produces
`TestNoSolderNameRemains` over the whole repository, with the history
allowlist.

- [ ] Widen `TestNoSolderNameRemains` to
      `git grep -n -i solder -- . ':!.procoder' ':!CHANGELOG.md' ':!go.sum'`.
      Treat any match as a failure, except lines in
      `kuvryn-sync-full-spec.md` after the `## History` heading and the section
      in `docs/upgrade.md` titled "Moving from Solder 0.3.x to Kuvryn Sync
      0.4.0". Run it, and expect it to FAIL, listing the documentation files.
- [ ] Rewrite the files listed under Files. Run
      `procoder format <file> > <file>.formatted && mv <file>.formatted <file>`
      for each Markdown file.
- [ ] Run `go test ./internal/brand/ && make test && make lint`, and expect
      PASS. Run the relative-link check
      `python3 hack/check-links.py docs README.md`, created in this task from the
      script used in the 0.3.0 audit, and expect 0 broken links.
- [ ] Commit "Rewrite the documentation for Kuvryn Sync".

## Task 10: Open, review, and merge the rename, then rename the repository

Files:

- The GitHub repository `azrtydxb/solder` is renamed `azrtydxb/kuvryn-sync`.
- The local `git remote` URL.
- `.github/workflows/*.yml`: only if a repository-name reference remains.

Interfaces: consumes the merged rename branch. Produces the repository
`azrtydxb/kuvryn-sync` with green CI.

- [ ] Run the fresh-context pre-PR review (REVIEW.md rubric plus the simplify
      lens) on the branch. Fix Critical and Important findings, then open the PR
      "Rename Solder to Kuvryn Sync".
- [ ] Merge only after Tests, Lint, E2E and Image are green and every review
      thread is answered.
- [ ] Run `gh repo rename kuvryn-sync -R azrtydxb/solder --yes`. Then run
      `git remote set-url origin https://github.com/azrtydxb/kuvryn-sync.git`
      and `gh repo view azrtydxb/kuvryn-sync --json name --jq .name`. Expect
      `kuvryn-sync`.
- [ ] Push an empty-change CI trigger if no run started. Expect Tests, Lint,
      E2E and Image to be green on `azrtydxb/kuvryn-sync`.

## Task 11: Release v0.4.0 and make the image public

Files:

- `charts/kuvryn-sync/Chart.yaml`: `version: 0.4.0` and `appVersion: 0.4.0`.
- `dist/install.yaml`: regenerated with
  `make build-installer IMG=ghcr.io/azrtydxb/kuvryn-sync:v0.4.0`.
- `CHANGELOG.md`: `## Unreleased` becomes `## 0.4.0`, with a summary line.
- `SECURITY.md`: the supported version becomes `v0.4.x`.
- `test/release/public_image_test.go`: new, behind the `release` build tag.

Interfaces: consumes the repository from Task 10. When the console plan is
executing, this task runs after that plan's last task, so v0.4.0 ships both.

- [ ] Write `test/release/public_image_test.go` (`//go:build release`):
  ```go
  func TestReleaseImageIsPublic(t *testing.T) {
  	version := os.Getenv("RELEASE_VERSION")
  	resp, err := http.Get("https://ghcr.io/token?scope=repository:azrtydxb/kuvryn-sync:pull")
  	if err != nil {
  		t.Fatal(err)
  	}
  	var tok struct{ Token string }
  	_ = json.NewDecoder(resp.Body).Decode(&tok)
  	req, _ := http.NewRequest("GET", "https://ghcr.io/v2/azrtydxb/kuvryn-sync/manifests/"+version, nil)
  	req.Header.Set("Authorization", "Bearer "+tok.Token)
  	req.Header.Set("Accept", "application/vnd.oci.image.index.v1+json, application/vnd.oci.image.manifest.v1+json")
  	res, err := http.DefaultClient.Do(req)
  	if err != nil {
  		t.Fatal(err)
  	}
  	if res.StatusCode != http.StatusOK {
  		t.Fatalf("anonymous pull of %s returned %d; make the package public", version, res.StatusCode)
  	}
  }
  ```
  Run `RELEASE_VERSION=v0.4.0 go test -tags release ./test/release/` before
  release, and expect it to FAIL with "returned 401" or "returned 403".
- [ ] Prepare the release on a branch: the Files above, then
      `procoder release 0.4.0` until it reports "ready". Open the PR, merge it,
      run `git tag -a v0.4.0 -m "0.4.0" && git push origin v0.4.0`, and create
      the GitHub release with the 0.4.0 changelog as notes and `dist/install.yaml`
      attached.
- [ ] Wait for the tag's Image run to succeed on `arc-azrtydxb-publish`. Then
      make the package public in the GitHub web UI, using a browser with the
      maintainer's session: package settings, Danger Zone, change visibility,
      Public.
- [ ] Run `RELEASE_VERSION=v0.4.0 go test -tags release ./test/release/`, and
      expect PASS. Run the image once on kw with `ksync version`, and expect
      `ksync v0.4.0`.
