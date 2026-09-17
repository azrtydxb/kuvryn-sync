# Security baseline blocks obvious leaks

Status: done
Created: 2026-09-17
Epic: ci-security-and-local-kind
Sprint: 001-phase-0-closeout-source-render-kickoff

## Description

Security baseline blocks obvious leaks.

## Acceptance criteria

- [x] Secret scanning runs in the gate.
- [x] Dockerfile linting runs.
- [x] Dependencies are scanned and actionable vulnerabilities are tracked.

## Evidence

- Evidence: `procoder security` reports 0 findings. Dockerfile lint/security tooling is installed per procoder doctor, dependency blockers were upgraded, and `.gitignore` excludes procoder local state/cache.
