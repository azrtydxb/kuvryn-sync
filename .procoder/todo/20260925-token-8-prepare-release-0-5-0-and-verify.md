# Token 8: Prepare release 0.5.0 and verify

Status: closed 2026-09-25
Created: 2026-09-25
Plan: .procoder/plans/console-token-signin.md

## Description

Version the chart and changelog as 0.5.0, regenerate `dist/install.yaml`,
run every suite and the gate, review the diff for token-handling defects,
and push the branch without opening a PR.

## Acceptance criteria

- [x] `CHANGELOG.md` has `## 0.5.0` with an italic summary, and the chart's version and appVersion are 0.5.0
- [x] `procoder release 0.5.0` reports ready
- [x] `make test`, `make lint`, `make test-ui`, `make docs-check` and `helm lint` pass
- [x] `procoder check` over the branch's changed files has no blocking findings
- [x] The security self-review's findings are fixed, and the branch is pushed

## Evidence

- dfc28f6 adds `## 0.5.0` with an italic summary and sets Chart.yaml version and appVersion to 0.5.0. `make build-installer IMG=ghcr.io/azrtydxb/kuvryn-sync:v0.5.0`, then formatting, changes only the image line in dist/install.yaml.
- `procoder release 0.5.0`: "release 0.5.0 is ready". Not tagged.
- Final runs:
  - `make test` exit 0.
  - `make lint`: 0 issues.
  - `make test-ui` exit 0: 10 passed.
  - `make docs-check`: 0 broken.
  - `helm lint`: 0 failed.
- `procoder check --paths-from` (git diff --name-only origin/main...HEAD): 48 clean, 0 unformatted, 0 blocking.
- Security self-review by a fresh-context reviewer found 0 Critical and 3 Minor.
  - Fixed in e27e21a: the token following redirects to other hosts, and unscrubbed read-error logs plus a 502 for ErrNotTheSessionToken.
  - Not fixed: rate limiting, which the spec puts out of scope.
  - The copied-cookie note was added to docs/security.md.
- The branch is pushed to origin/feat/console-token-signin with no PR.
