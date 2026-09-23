# Image automation

Status: done 2026-09-23
Created: 2026-09-23
Milestone: v0-3-regulated-delivery

## Description

Solder watches Git only, so it cannot tell when CI has built a new image — and not every commit produces one. This epic lets Solder scan container registries, select the image to run by policy, and commit the resolved digest back to Git, so Git stays the source of truth and every image bump flows through the normal plan, approval, and Revision audit path. Decision recorded 2026-09-23: built-in automation with Git write-back, not in-cluster overrides; placed in v0.3.
