# Console 3: Read the cluster as the user, read-only

Status: closed 2026-09-25
Created: 2026-09-25
Plan: .procoder/plans/kuvryn-sync-console.md

## Description

UserClient with impersonation and readOnlyTransport, which refuses writes, Secrets and system: identities.

## Acceptance criteria

- [x] `TestConsoleClientIsReadOnly` fails first, then passes
- [x] `procoder check` has no blocking findings for the resulting change.

## Evidence

- `go test ./internal/console/ -run TestConsoleClientIsReadOnly` before: FAIL to compile, `kube_test.go:26:17: undefined: readOnlyTransport`; after: `--- PASS: TestConsoleClientIsReadOnly`.
- Mutation check: disabling the GET check failed it with `POST allowed`. Disabling the Secret path check did not fail that test alone (the unimpersonated GET is already refused), so `TestUserClientSendsOnlyImpersonatedGETs` (every request reaching a fake API server is a GET impersonating `alice` / `team-a`, and Secret get and list return `ErrForbiddenPath` without reaching it) and `TestForbiddenPaths` were added; both fail under that mutation.
- `make test` exit 0; `make lint` first reported revive `import-shadowing` (`rest`), fixed, then "0 issues."; `make manifests generate` left no diff.
- `procoder check`: 0 blocking.
