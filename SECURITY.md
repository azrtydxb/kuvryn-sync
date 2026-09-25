# Security Policy

Kuvryn Sync is alpha software. Please report security issues privately to the
repository owner instead of opening a public issue with exploit details.

## Supported versions

| Version  | Supported               |
| -------- | ----------------------- |
| `v0.4.x` | Best-effort alpha fixes |

## Sensitive data expectations

Kuvryn Sync is designed to redact Secret values from status, CLI output, Events, logs, traces,
metrics, and diagnostics. If you find a path that leaks sensitive data, treat it
as a security issue.
