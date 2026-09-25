# Token 2: Read the cluster with the session's own token

Status: open
Created: 2026-09-25
Plan: .procoder/plans/console-token-signin.md

## Description

A token session must read as the pasted token, never as the console's
ServiceAccount and never through impersonation. Build its client from the
base config with every console credential stripped, and make the read-only
transport refuse `Impersonate-*` headers and any other bearer token in token
mode, while keeping its GET-only, Secret and subresource rules (spec S-2).

## Acceptance criteria

- [ ] `TestTokenSessionReadsAsTheToken` failed before the change and passes after it
- [ ] No request carries the console's bearer token, basic auth, client certificate, exec or auth-provider credentials, or an `Impersonate-*` header
- [ ] Writes, Secret reads and subresource requests never leave the process
- [ ] The existing read-only transport tests still pass

## Evidence
