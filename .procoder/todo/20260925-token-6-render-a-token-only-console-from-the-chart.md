# Token 6: Render a token-only console from the chart

Status: closed 2026-09-25
Created: 2026-09-25
Plan: .procoder/plans/console-token-signin.md

## Description

Make `console.oidc.issuerURL` and `console.oidc.clientID` optional but
paired. Without them, render no impersonate ClusterRole or binding, no OIDC
flags and no redirect-URL requirement. With them, render exactly what 0.4.2
rendered (spec S-6).

## Acceptance criteria

- [x] `TestConsoleChartTokenOnly` failed before the change and passes after it
- [x] Console objects rendered with OIDC values match golden renders of v0.4.2's chart
- [x] Only one of the two OIDC values fails the render
- [x] `helm lint charts/kuvryn-sync` passes

## Evidence

- Red before 20ac3ad: `TestConsoleChartTokenOnly` failed with "console.redirectURL is required" for a console with no OIDC values.
- Green: the token-only render has no ClusterRole, ClusterRoleBinding or impersonate rule, and no --oidc-, --redirect-url or claim flags. A lone issuer or client ID fails with "must be set together".
- Three OIDC value sets match goldens rendered from v0.4.2's chart. Since a5c1c10 they are compared by content, and changing one value fails the test.
- `helm template … --show-only templates/console.yaml` with OIDC values is byte-identical to v0.4.2's chart (diff exit 0).
- `helm lint charts/kuvryn-sync` passes, and so does the same with `--set console.enabled=true`: 1 chart linted, 0 failed.
