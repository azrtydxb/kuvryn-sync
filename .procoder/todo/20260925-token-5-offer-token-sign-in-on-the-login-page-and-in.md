# Token 5: Offer token sign-in on the login page and in console-dev

Status: closed 2026-09-25
Created: 2026-09-25
Plan: .procoder/plans/console-token-signin.md

## Description

Add a "Sign in with a Kubernetes token" form to the login page with the
vendored Azrty components, show OIDC buttons only when OIDC is configured,
and let hack/console-dev mint ServiceAccount tokens so Playwright can sign in
with a real token (spec S-1, S-3).

## Acceptance criteria

- [x] `TestLoginPage` (Playwright) failed before the change and passes after it
- [x] The form uses a password-type input and shows the hint `kubectl create token <serviceaccount> -n <namespace>`
- [x] OIDC buttons are hidden without OIDC and shown with it
- [x] A token sign-in against hack/console-dev lands on `/apps` as the ServiceAccount
- [x] `make test-ui`, the typecheck and the unit tests pass

## Evidence

- Red before 6709d7f: `make test-ui` failed every TestLoginPage case ("waiting for getByLabel('Kubernetes token')"), and `go run ./hack/console-dev token` did not exist.
- Green: `make test-ui` exit 0, 10 passed, 3 skipped (screenshots). This covers:
  - the token form, with type=password and the exact hint, in dark and light at 1400 and 700 px;
  - no Dex or GitHub buttons without OIDC;
  - the OIDC buttons when /api/me reports OIDC;
  - a real ServiceAccount token landing on /apps as `system:serviceaccount:default:console-viewer`;
  - a bad token landing on /login?error=token.
- `npm --prefix web run typecheck` is clean and `npm --prefix web test` passes (3 tests).
- The OIDC contrast check was stabilised in 1536eca (it waits for the button transition), then passed 15 times in a row with `--repeat-each 15`.
