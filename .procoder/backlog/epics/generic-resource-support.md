# Generic resource support

Status: done 2026-09-23
Created: 2026-09-23
Milestone: v0-2-tenancy-and-generic-resources

## Description

Solder handles any Kubernetes kind the Application's service account can manage: it watches applied kinds for drift, evaluates health with kstatus conventions, and lets operators define CEL health rules for kinds kstatus cannot judge. Replaces the hard-coded kind list in watches, health, and RBAC. Decision recorded 2026-09-23: kstatus plus user-defined CEL rules.
