# Charts from HTTP and OCI registries

Status: open
Created: 2026-09-23
Epic: helm-chart-sources
Sprint: -

## Description

As an application team, I deploy a third-party chart by repository URL, chart name, and version.

## Acceptance criteria

- [ ] Helm render spec accepts an HTTP or `oci://` chart reference with a pinned version; registry credentials come from a Secret.
- [ ] Chart downloads are cached by digest; the Revision records the resolved chart digest.
- [ ] Charts with dependencies render (`helm dependency build` or equivalent).

## Evidence

