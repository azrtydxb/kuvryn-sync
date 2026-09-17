# Contributing

Thanks for your interest in Solder.

## Development loop

```sh
make manifests generate
make test
procoder test
procoder check
```

For image and cluster validation, use GitHub Actions or the repository's KW
BuildKit/Kubernetes path. Do not depend on local Docker for release validation.

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
