# Rebrand 11: Release v0.4.0 and make the image public

Status: closed 2026-09-25
Created: 2026-09-25
Plan: .procoder/plans/kuvryn-sync-rebrand.md

## Description

Chart 0.4.0, the installer, CHANGELOG 0.4.0, the tag, the GitHub release and GHCR visibility public. This runs after the console plan.

## Acceptance criteria

- [x] `TestReleaseImageIsPublic` fails while the package is private (the maintainer decided on 2026-09-25 to keep it private for now)
- [x] The v0.4.0 image run on kw prints `ksync v0.4.0`
- [x] `procoder check` has no blocking findings for the resulting change.

## Evidence

- PR #15 merged as 82a971a; tag v0.4.0 pushed; GitHub release https://github.com/azrtydxb/kuvryn-sync/releases/tag/v0.4.0 with dist/install.yaml attached.
- Tag Image run 36165839537: success.
- Decision (maintainer, 2026-09-25): the GHCR package stays private for now; clusters pull with an existing GHCR token. `RELEASE_VERSION=v0.4.0 go test -tags release ./test/release/` -> "anonymous pull of v0.4.0 returned 404; make the package public", as expected while private. The chart gained `image.pullSecrets` (0.4.1) so a private image installs without hand patches.
- kw, throwaway namespace ksync-release-check with the existing ghcr-pull secret: `ksync version` -> `ksync v0.4.0` (image ghcr.io/azrtydxb/kuvryn-sync@sha256:f738a6b7b3b6c9d027388570ebf4c128dfb0a7ec2b2b666ec9ceb54f315b442d); namespace deleted.

- Release PR #15 merged as 82a971a; tag v0.4.0 pushed; GitHub release https://github.com/azrtydxb/kuvryn-sync/releases/tag/v0.4.0 with dist/install.yaml attached.
- Tag Image run 36165839537 succeeded on arc-azrtydxb-publish.
- `RELEASE_VERSION=v0.4.0 go test -tags release ./test/release/` -> "anonymous pull of v0.4.0 returned 404; make the package public": the package stays private by the maintainer's decision (2026-09-25, "no need to make it public now, you can use our existing gh tokens").
- kw: in a throwaway namespace with the existing ghcr-pull secret, `ghcr.io/azrtydxb/kuvryn-sync:v0.4.0 version` printed `ksync v0.4.0` (image sha256:f738a6b7b3b6c9d027388570ebf4c128dfb0a7ec2b2b666ec9ceb54f315b442d); the namespace was deleted.
- Follow-up from the decision: the chart gains `image.pullSecrets` in 0.4.1.

