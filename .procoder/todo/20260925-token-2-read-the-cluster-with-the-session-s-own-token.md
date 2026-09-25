# Token 2: Read the cluster with the session's own token

Status: closed 2026-09-25
Created: 2026-09-25
Plan: .procoder/plans/console-token-signin.md

## Description

A token session must read as the pasted token, never as the console's
ServiceAccount and never through impersonation. Build its client from the
base config with every console credential stripped, and make the read-only
transport refuse `Impersonate-*` headers and any other bearer token in token
mode, while keeping its GET-only, Secret and subresource rules (spec S-2).

## Acceptance criteria

- [x] `TestTokenSessionReadsAsTheToken` failed before the change and passes after it
- [x] No request carries the console's bearer token, basic auth, client certificate, exec or auth-provider credentials, or an `Impersonate-*` header
- [x] Writes, Secret reads and subresource requests never leave the process
- [x] The existing read-only transport tests still pass

## Evidence

- Red: `TestTokenSessionReadsAsTheToken` failed to build before the change (`undefined: TokenClient`, `undefined: ErrNotTheSessionToken`).
- Green in af8f097. A TLS API server that requests client certs sees only `Authorization: Bearer user-token`, no `Impersonate-*` header and no peer certificate, although the base config sets a bearer token, a token file, a client cert, basic auth, Impersonate, an ExecProvider, an AuthProvider and a WrapTransport.
- The same test shows Create, Delete, a Secret get and list, and pods/log and pods/status failing with ErrWriteRefused or ErrForbiddenPath without reaching the server. `TestTokenTransportRefusesOtherCredentials` passes too.
- `TestUserClientSendsOnlyImpersonatedGETs`, `TestForbiddenPaths` and `TestConsoleClientIsReadOnly` still PASS.
- Later, e27e21a pins the transport to the API server's origin. `TestTokenNeverFollowsARedirectElsewhere` was red first ("the token reached another host 2 times") and is green now.
