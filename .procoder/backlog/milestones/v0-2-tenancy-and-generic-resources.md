# v0.2 Tenancy and generic resources

Status: done 2026-09-23
Created: 2026-09-23

## Goal

Solder is safe to run with more than one team on a cluster: an Application can only change what its service account may change, and any Kubernetes kind — including CRDs from other operators — can be applied, watched for drift, and health-checked.

## Success state

- A tenant with rights to create an Application cannot use Solder to create objects their own RBAC forbids, proven by an e2e test.
- An Application managing a non-built-in CRD kind syncs, detects drift, and reports health without hanging until timeout.
