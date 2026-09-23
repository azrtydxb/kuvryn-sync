# SOPS decryption

Status: open
Created: 2026-09-23
Milestone: v0-3-regulated-delivery

## Description

Secrets encrypted with SOPS (age keys first) can live in Git and are decrypted in the controller at render time, never written to plans, status, Events, or logs.
