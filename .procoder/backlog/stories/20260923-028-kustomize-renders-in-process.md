# Kustomize renders in process

Status: done 2026-09-23
Created: 2026-09-23
Epic: in-process-sources-and-renderers
Sprint: -

## Description

As an application team, my `render.type: kustomize` Applications render in the published controller image, without a kustomize binary, and cannot read files outside the Git checkout.

## Acceptance criteria

- [x] `KustomizeRenderer` uses sigs.k8s.io/kustomize/api (krusty) instead of executing `kustomize build`.
- [x] Rendering runs on an in-memory copy of the workspace, so bases or resources outside the checkout fail; a unit test proves a `../` escape is rejected.
- [x] Unit tests cover an overlay over a base with a patch.

## Evidence

- `internal/renderer/kustomize` runs krusty (kustomize/api v0.21.1) on an in-memory copy of the workspace; the exec renderer package is deleted.
- `TestRenderOverlayWithPatch` renders an overlay over a base with a patch; `TestRenderCannotReadOutsideWorkspace` fails when rendering on the real disk (mutation checked); `TestRenderRejectsSymlinkOutOfWorkspace` and `TestRenderRejectsTraversalPath` cover symlink and path escapes.
