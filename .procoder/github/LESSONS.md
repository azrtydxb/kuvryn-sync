# Lessons — findings that escaped our own gates

One entry per finding caught downstream (bot review, human review,
production) — the escape is the bug; the finding is its symptom. Every
entry names which layer should have caught it and the adaptation that now
does. `procoder lessons` flags entries with no adaptation.

Entry shape (unindented in real entries):

    ## <date> <where caught> — <one-line finding>

    - Class: mechanical | judgment | taste
    - Missed by: linter | rubric | controller | test | ci
    - Adaptation: <the concrete change that catches this class from now on>

## 2026-09-23 Copilot review of PR #3 — render path through a symlink escaped containment

- Class: mechanical
- Missed by: rubric
- Adaptation: REVIEW.md now requires containment checks to cover the starting path itself; `renderer.Contained` checks `dir` and `TestContainedRefusesARenderPathThroughAnEscapingLink` pins it.

## 2026-09-23 Copilot review of PR #3 — system:anonymous accepted as an approver

- Class: judgment
- Missed by: rubric
- Adaptation: REVIEW.md now lists anonymous and unauthenticated identities as unauthenticated; the webhook test covers `system:anonymous` and the `system:unauthenticated` group.

## 2026-09-23 Copilot review of PR #3 — chart Services selected every release's pods

- Class: mechanical
- Missed by: test
- Adaptation: `TestHelmChartServicesSelectTheirOwnRelease` fails when a chart Service does not select `app.kubernetes.io/instance`, and REVIEW.md asks for per-release selectors.

## 2026-09-23 Copilot review of PR #3 — fork pull requests would run on the lab's self-hosted runners

- Class: judgment
- Missed by: rubric
- Adaptation: REVIEW.md now forbids running fork pull requests on self-hosted runners; the workflows send fork PRs to GitHub-hosted runners.

## 2026-09-23 Copilot review of PR #4 — Go bump missed the devcontainer image

- Class: mechanical
- Missed by: test
- Adaptation: `TestGoVersionMatchesBuildImages` fails when the Dockerfile or devcontainer image lags the go.mod Go version.

## 2026-09-23 Copilot review of PR #4 — trigger comment copied into workflows it did not describe

- Class: taste
- Missed by: rubric
- Adaptation: REVIEW.md now asks for every copied comment to be re-read against the file it lands in.

## 2026-09-23 Copilot review of PR #6 — prune stopped at the first checkout it could not remove

- Class: mechanical
- Missed by: rubric
- Adaptation: REVIEW.md now asks for every instance of a fixed pattern to be fixed; the pre-PR review had flagged the outer loop only. `TestPruneKeepsGoingPastACheckoutItCannotRemove` pins the inner loop.

## 2026-09-24 Copilot review of PR #8 — managed PodDisruptionBudgets never listed their Pods

- Class: mechanical
- Missed by: test
- Adaptation: `TestCollectReadsThePodsABudgetCovers` pins that a managed PDB alone makes Collect read its Pods; the edge tests had built graphs from Pods handed in directly, never through Collect.
