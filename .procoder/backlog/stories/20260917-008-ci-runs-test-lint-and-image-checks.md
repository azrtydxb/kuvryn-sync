# CI runs test lint and image checks

Status: done
Created: 2026-09-17
Epic: ci-security-and-local-kind
Sprint: 001-phase-0-closeout-source-render-kickoff

## Description

CI runs test lint and image checks.

## Acceptance criteria

- [x] GitHub workflows run unit tests and lint on pull requests.
- [x] Jobs have timeouts and concurrency cancellation.
- [x] Container build path is verified.

## Evidence

- Evidence: `.github/workflows/test.yml`, `lint.yml`, `test-e2e.yml`, and `image.yml` have timeouts and concurrency cancellation. Test/lint workflows run Go tests and lint; image workflow runs `docker build -t solder:ci .`; `procoder check` has no blocking CI findings.
