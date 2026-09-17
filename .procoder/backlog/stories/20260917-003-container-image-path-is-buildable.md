# Container image path is buildable

Status: done
Created: 2026-09-17
Epic: foundation-control-plane
Sprint: 010-phase-8-packaging-docs-kw-validation
Carried: 001-phase-0-closeout-source-render-kickoff — resolved by KW cluster BuildKit evidence; local Docker/locker Docker is intentionally not used.

## Description

Container image path is buildable.

## Acceptance criteria

- [x] Dockerfile builds the manager image.
- [x] Image tag is configurable through `IMG`.
- [x] Build context excludes generated/cache junk.

## Evidence

- Evidence: Dockerfile built successfully through the KW cluster BuildKit service with `buildctl --addr tcp://192.168.10.130:1234 ... --opt platform=linux/arm64 --output type=image,name=192.168.10.131:5000/solder:ci,push=false`. No local Docker/locker Docker was used. `IMG` remains configurable through the existing Makefile, and `.dockerignore` limits build context.

- Correction: closure evidence is the KW cluster BuildKit image build recorded above. Local Docker/locker Docker evidence is intentionally not required for this repository.
