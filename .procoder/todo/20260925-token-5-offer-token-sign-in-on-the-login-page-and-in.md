# Token 5: Offer token sign-in on the login page and in console-dev

Status: open
Created: 2026-09-25
Plan: .procoder/plans/console-token-signin.md

## Description

Add a "Sign in with a Kubernetes token" form to the login page with the
vendored Azrty components, show OIDC buttons only when OIDC is configured,
and let hack/console-dev mint ServiceAccount tokens so Playwright can sign in
with a real token (spec S-1, S-3).

## Acceptance criteria

- [ ] `TestLoginPage` (Playwright) failed before the change and passes after it
- [ ] The form uses a password-type input and shows the hint `kubectl create token <serviceaccount> -n <namespace>`
- [ ] OIDC buttons are hidden without OIDC and shown with it
- [ ] A token sign-in against hack/console-dev lands on `/apps` as the ServiceAccount
- [ ] `make test-ui`, the typecheck and the unit tests pass

## Evidence
