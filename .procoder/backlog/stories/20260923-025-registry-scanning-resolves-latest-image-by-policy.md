# Registry scanning resolves the latest image by policy

Status: done 2026-09-23
Created: 2026-09-23
Epic: image-automation
Sprint: -

## Description

As an application team, I declare which image repository to watch and a selection policy, and Solder tells me the image and digest that currently satisfies it — so a new build is detected even when no Git commit happened.

## Acceptance criteria

- [x] An image policy names a registry repository, pull credentials from a Secret (labelled `solder.io/registry-credentials=true`, like Git credentials), a scan interval, and one policy: semver range, tag regex with ordering, or latest digest of a fixed tag.
- [x] Status reports the selected tag and its immutable digest, plus the last scan time; scan failures are conditions with redacted messages.
- [x] OCI registries (GHCR, Docker Hub, generic Distribution) are covered by unit tests against a fake registry.
- [x] Scans are rate-limited per registry and emit a metric for failures.

## Evidence

- API (compact shape, decided 2026-09-23): `kubebuilder create api --kind ImagePolicy --controller`; spec image, secretRef, interval (default 5m), policy {semver | tagPattern{regex, order} | digest{tag}} with a CEL rule enforcing exactly one (`rejects a policy that sets more than one selector`).
- Selection: `TestSelect` covers semver range (prereleases excluded), numerical ordering by capture group, alphabetical ordering, digest-follows-tag, and no match.
- Registry: oras-go (already in the tree via Helm) lists tags and resolves digests with dockerconfigjson credentials; `TestRegistryListsTagsAndResolvesDigestsWithCredentials` runs against an in-test OCI registry requiring basic auth, including a 401 with wrong credentials.
- Controller: `selects the newest matching tag by digest and picks up a newly pushed build` asserts `status.latestImage = image:tag@digest`, `lastScannedAt`, requeue at the interval, and a new tag being selected on the next scan; `refuses registry credentials that are not labelled for registries` asserts a `ScanFailed` condition (fails without the label check, mutation checked).
- Rate limiting: every registry request waits on a limiter shared per registry host (default 5/s, burst 10) across all ImagePolicies; `TestRegistryRateLimitsPerHost` proves a second repository on the same host cannot bypass it. Metric: `solder_image_scans_total{result}`.
- Gates: `make test`, `make lint` 0 issues, `procoder check`/`security` clean.
