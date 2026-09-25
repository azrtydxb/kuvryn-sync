# Token 8: Prepare release 0.5.0 and verify

Status: open
Created: 2026-09-25
Plan: .procoder/plans/console-token-signin.md

## Description

Version the chart and changelog as 0.5.0, regenerate `dist/install.yaml`,
run every suite and the gate, review the diff for token-handling defects,
and push the branch without opening a PR.

## Acceptance criteria

- [ ] `CHANGELOG.md` has `## 0.5.0` with an italic summary, and the chart's version and appVersion are 0.5.0
- [ ] `procoder release 0.5.0` reports ready
- [ ] `make test`, `make lint`, `make test-ui`, `make docs-check` and `helm lint` pass
- [ ] `procoder check` over the branch's changed files has no blocking findings
- [ ] The security self-review's findings are fixed, and the branch is pushed

## Evidence
