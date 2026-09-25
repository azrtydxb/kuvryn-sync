# Console 1: Serve ksync console with health and an embedded placeholder

Status: closed 2026-09-25
Created: 2026-09-25
Plan: .procoder/plans/kuvryn-sync-console.md

## Description

internal/console config and server, the ui embed with placeholder, and the ksync console subcommand.

## Acceptance criteria

- [x] `TestServerServesHealthAndSecurityHeaders` fails first, then passes
- [x] `ksync console --help` lists every flag
- [x] `procoder check` has no blocking findings for the resulting change.

## Evidence

- `go test ./internal/console/` before: FAIL to compile, `internal/console/server_test.go:12:12: undefined: NewServer`; after: `ok` (`TestServerServesHealthAndSecurityHeaders`: SPA fallback 200 with `<html`, CSP has `frame-ancestors 'none'`, `/healthz` 503 before OIDC).
- `go run ./cmd console --help` printed all 15 flags; `TestConsoleHelpListsEveryFlag` in `internal/cli` asserts each one and the claim, listen and cluster defaults: `ok`.
- `make test` exit 0; `make lint`: "0 issues."; `make manifests generate` left `config/` and `api/` unchanged.
- `procoder check`: 0 blocking.
