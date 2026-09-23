# Registry scanning resolves the latest image by policy

Status: open
Created: 2026-09-23
Epic: image-automation
Sprint: -

## Description

As an application team, I declare which image repository to watch and a selection policy, and Solder tells me the image and digest that currently satisfies it — so a new build is detected even when no Git commit happened.

## Acceptance criteria

- [ ] An image policy names a registry repository, pull credentials from a Secret, a scan interval, and one policy: semver range, tag regex with ordering, or latest digest of a fixed tag.
- [ ] Status reports the selected tag and its immutable digest, plus the last scan time; scan failures are conditions with redacted messages.
- [ ] OCI registries (GHCR, Docker Hub, generic Distribution) are covered by unit tests against a fake registry.
- [ ] Scans are rate-limited per registry and emit a metric for failures.

## Evidence

