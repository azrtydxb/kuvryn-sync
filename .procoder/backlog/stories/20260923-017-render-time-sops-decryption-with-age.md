# Render-time SOPS decryption with age

Status: done 2026-09-23
Created: 2026-09-23
Epic: sops-decryption
Sprint: -

## Description

As an application team, I commit SOPS-encrypted Secret manifests and Solder applies the decrypted Secret.

## Acceptance criteria

- [x] Application references a decryption key Secret; age keys are supported.
- [x] Decryption happens after render and before validation; decrypted values never reach plan output, and a test asserts this.
- [x] Decryption failure fails the Revision with a redacted, non-retryable reason.
- [x] Works for yaml and kustomize renderers; Helm behaviour is specified (values files are not decrypted; documented).

## Evidence

- `internal/decrypt` uses the official sops/v3 library (decided 2026-09-23) with an in-process key service that answers only age requests from the Application's keys, so no process-wide SOPS environment is used; MAC verification via `common.DecryptTree`.
- `spec.decryption{provider: sops, secretRef}`; the key Secret must be labelled `solder.io/decryption-key=true`; `.agekey` entries are used.
- Unit: `TestDecryptsWithTheMatchingAgeKey`; `TestRejectsWrongKeyTamperingAndMissingDecryption` (wrong key, MAC failure after renaming the object, nil decryptor refuses ciphertext, plain files unchanged). Kustomize: `TestRenderDecryptsSOPSBeforeTransforms` decrypts before `namePrefix` (which would otherwise break the MAC).
- Envtest: `applies the decrypted Secret and never shows plaintext in the plan` (live Secret holds the plaintext, Revision status does not; fails when the renderer gets no decryptor, mutation checked); `refuses to apply ciphertext when decryption is not configured` (non-retryable path, nothing applied).
- Helm: values files are not decrypted; documented in operations.md.
- Gates: `make test`, `make lint` 0 issues, `procoder check`/`security` clean. Manager binary grows from 118 MB to 164 MB.
