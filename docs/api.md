# Solder API

Solder exposes Kubernetes CRDs in `solder.io/v1alpha1`:

- `Repository`: desired-state source and Secret-backed Git authentication reference.
- `Application`: sync, health, history, destination, and safety policy.
- `Revision`: auditable deployment attempt with bounded, redacted plan and health status.

Important API invariants:

- Sync state and health state are separate.
- Revision status stores bounded plan summaries and redacted per-resource changes.
- Server-side apply conflicts fail by default.
- Secret values must not appear in status, CLI output, Events, logs, metrics, or diagnostics.
- Rollback uses normal plan/apply/observe machinery and points at a previous Healthy Revision.
