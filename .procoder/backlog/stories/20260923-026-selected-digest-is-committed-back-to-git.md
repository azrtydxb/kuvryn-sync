# Selected digest is committed back to Git

Status: done 2026-09-23
Created: 2026-09-23
Epic: image-automation
Sprint: -

## Description

As an operator, I want a new image selection to become a Git commit, so the deploy is reproducible, reviewable, and reverted with `git revert` — never an in-cluster patch that makes the cluster disagree with Git.

## Acceptance criteria

- [x] Solder updates image references marked with Flux-compatible setter comments (decided 2026-09-23) in YAML files, including kustomize `images` entries and Helm values files, and pushes a commit whose message names the old and new reference for each change.
- [x] Write-back uses a separate write credential Secret; branch (decided 2026-09-23: a configurable branch, no PR API integration), author, and path are configurable.
- [x] No commit is made when the digest is unchanged; a test proves idempotence.
- [x] Push conflicts are retried with rebase, bounded, and reported as a condition when they fail.
- [x] The resulting commit flows through the normal Repository → Application → Revision path, including manual approval when enabled.

## Evidence

- `internal/imageupdate`: `Rewrite` handles `ns:name` (full `image:tag@digest`), `:tag`, and `:name` markers, quoted values, and leaves other namespaces' markers alone (`TestRewriteMarkers`, strengthened so dropping the namespace check fails, mutation checked). Markers are line comments, so they work in plain manifests, kustomize `images` entries, and Helm values files alike.
- Push: go-git clones the branch into memory (never the shared read-only checkout), commits as `spec.imageUpdate.author*`, and pushes with the labelled write-credential Secret; commit message lists `policy (file): old -> new` (`TestUpdateCommitsAndPushesOnce`).
- Idempotent: an unchanged selection makes no commit (`TestUpdateCommitsAndPushesOnce` second run; envtest second reconcile keeps the head).
- Conflicts: a push rejected because the branch moved is retried from a fresh clone, up to 3 times (`TestUpdateRetriesWhenTheBranchMoves` races a real concurrent push and keeps both commits); failures set `ImagesUpdated=False/UpdateFailed` and a Warning Event.
- Flow: `commits the selected image to Git once and reports it` drives the Repository reconciler: the commit lands in origin, `ImagesUpdated=Committed` names it, and the Repository re-fetches after 1s; the new commit then plans (and awaits approval if manual) like any other.
- Gates: `make test`, `make lint` 0 issues, `procoder check`/`security` clean.
