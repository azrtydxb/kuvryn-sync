# Git auth uses Kubernetes Secrets safely

Status: done
Created: 2026-09-17
Epic: git-source-and-cache
Sprint: 001-phase-0-closeout-source-render-kickoff

## Description

Git auth uses Kubernetes Secrets safely.

## Acceptance criteria

- [x] SSH keys, known_hosts, usernames, passwords, and tokens load from Secrets.
- [x] Missing or invalid credentials produce AuthenticationFailure.
- [x] No credential values are logged.

## Evidence

- Evidence: `internal/controller/repository_controller.go` loads username/password/token/SSH key/known_hosts from Kubernetes Secrets. Missing or unsupported auth Secrets produce AuthenticationFailure. Controller tests verify token values are passed to the resolver but not exposed in status condition messages.
