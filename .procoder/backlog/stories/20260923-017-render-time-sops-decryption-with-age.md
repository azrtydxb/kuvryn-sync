# Render-time SOPS decryption with age

Status: open
Created: 2026-09-23
Epic: sops-decryption
Sprint: -

## Description

As an application team, I commit SOPS-encrypted Secret manifests and Solder applies the decrypted Secret.

## Acceptance criteria

- [ ] Application references a decryption key Secret; age keys are supported.
- [ ] Decryption happens after render and before validation; decrypted values never reach plan output, and a test asserts this.
- [ ] Decryption failure fails the Revision with a redacted, non-retryable reason.
- [ ] Works for yaml and kustomize renderers; Helm behaviour is specified.

## Evidence

