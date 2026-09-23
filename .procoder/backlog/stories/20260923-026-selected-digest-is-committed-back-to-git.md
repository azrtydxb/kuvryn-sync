# Selected digest is committed back to Git

Status: open
Created: 2026-09-23
Epic: image-automation
Sprint: -

## Description

As an operator, I want a new image selection to become a Git commit, so the deploy is reproducible, reviewable, and reverted with `git revert` — never an in-cluster patch that makes the cluster disagree with Git.

## Acceptance criteria

- [ ] Solder updates image references marked in Git (a marker comment or a declared field path in YAML, kustomize `images`, or Helm values) and pushes a commit with a templated message naming the old and new digest.
- [ ] Write-back uses a separate write credential Secret; branch, author, and push target (direct or a separate branch for PRs) are configurable.
- [ ] No commit is made when the digest is unchanged; a test proves idempotence.
- [ ] Push conflicts are retried with rebase, bounded, and reported as a condition when they fail.
- [ ] The resulting commit flows through the normal Repository → Application → Revision path, including manual approval when enabled.

## Evidence

