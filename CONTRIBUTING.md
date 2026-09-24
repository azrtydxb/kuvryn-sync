# Contributing

Thanks for your interest in Solder.

## Toolchain

Solder needs Go 1.26 or later, as `go.mod` requires. The devcontainer in
`.devcontainer/` provides it on the `golang:1.26` image, with Docker-in-Docker,
kind, kubebuilder, and kubectl; open the repository in it to get a working
setup without installing anything locally.

## Development loop

```sh
make manifests generate
make test
procoder test
procoder check
```

For image and cluster validation, prefer GitHub Actions or another repeatable
remote build/test environment that matches your target cluster.

## CI

The Tests, Lint, E2E, and Image workflows run once per pull request commit and
on pushes to `main`; Image also runs on `v*` release tags and publishes from
`main` and tags. A newer pull request commit cancels the run in progress. Pull requests
from branches of this repository, and pushes to `main`, run on the project's
self-hosted lab runners. Pull requests from forks run untrusted code, so they
run on GitHub-hosted `ubuntu-latest` runners instead.

## Pull requests

A good PR should include:

- a clear problem statement;
- tests or an explanation of why tests are not appropriate;
- generated CRDs/deepcopy updates when API markers or types change;
- documentation updates for behavior visible to users/operators;
- no Secret material in examples, logs, or fixtures.

## Generated files

Do not edit generated files directly:

- `config/crd/bases/*.yaml`
- `config/rbac/role.yaml`
- `**/zz_generated.*.go`
- `PROJECT`

Regenerate them with `make manifests generate`.
