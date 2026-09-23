# v0.3 Regulated delivery

Status: open
Created: 2026-09-23

## Goal

Solder is the approval-gated GitOps controller for regulated teams: every change has a stored plan, an attributable approval bound to that plan, an outbound notification, and a documented dependency order, with secrets that never need to sit in Git as plaintext.

## Success state

- Approvals record who approved and are invalidated when the plan changes.
- Lifecycle notifications reach at least one sink without adding a broker.
- Applications can depend on other Applications, SOPS-encrypted Secrets decrypt at render time, and Git pushes trigger reconciles through a webhook.
