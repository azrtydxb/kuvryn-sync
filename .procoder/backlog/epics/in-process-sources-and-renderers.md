# In-process sources and renderers

Status: open
Created: 2026-09-23
Milestone: v0-2-tenancy-and-generic-resources

## Description

Solder shelled out to git, kustomize, and helm, but the controller image only shipped git, so kustomize and helm Applications could not render in the published image. This epic replaces all three with in-process libraries (kustomize/api, the Helm SDK, go-git), so the image needs no external binaries, and confines every read to the checked-out workspace. Decision recorded 2026-09-23: libraries for all three.
