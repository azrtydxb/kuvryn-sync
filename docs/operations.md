# Solder operations

Solder is installed from generated CRDs plus the controller manifests in `config/` or the alpha Helm chart in `charts/solder`.

Core commands use Kubernetes CRDs directly:

- `solder plan <application>` reads Revision status and prints redacted plan output.
- Application, Repository, and Revision status remain the public integration API.
- Mutation helpers produce public Application patches and require exact Revision approval.

Safety defaults:

- Server-side apply conflicts fail by default.
- Secret values are redacted from plans and CLI output.
- Sync state and health state are separate.
- Leader election is available through `--leader-elect` for HA deployments.

Build/test policy for this repository: use the KW cluster BuildKit and Kubernetes API for image and cluster validation. Do not use local Docker.
