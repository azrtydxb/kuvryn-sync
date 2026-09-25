# Token 6: Render a token-only console from the chart

Status: open
Created: 2026-09-25
Plan: .procoder/plans/console-token-signin.md

## Description

Make `console.oidc.issuerURL` and `console.oidc.clientID` optional but
paired. Without them, render no impersonate ClusterRole or binding, no OIDC
flags and no redirect-URL requirement. With them, render exactly what 0.4.2
rendered (spec S-6).

## Acceptance criteria

- [ ] `TestConsoleChartTokenOnly` failed before the change and passes after it
- [ ] Console objects rendered with OIDC values match golden renders of v0.4.2's chart
- [ ] Only one of the two OIDC values fails the render
- [ ] `helm lint charts/kuvryn-sync` passes

## Evidence
