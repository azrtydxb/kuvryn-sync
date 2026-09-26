# Console screenshots

Captured from the live console on the kw cluster (Kuvryn Sync 0.6.4), signed
in with a read-only viewer token, at a 1440×900 viewport and 2x scale
(2880×1800 files). The cluster had three Applications: `kuvryn-scout`, which
is healthy, and two demo Applications in the `ksync-demo` namespace. `podinfo`
has a plan waiting for manual approval, and `podinfo-broken` is Degraded
because of a missing Secret. The CLI screenshots in `../cli/` come from the
same session. Retake the screenshots after UI changes rather than editing them.

| File                                              | Shows                                                     |
| ------------------------------------------------- | --------------------------------------------------------- |
| `login-dark.png`, `login-light.png`               | Token sign-in page, dark and light theme                  |
| `login-error.png`                                 | The message after an invalid token                        |
| `applications-dark.png`, `applications-light.png` | Applications: Degraded banner, stat cards, filters, table |
| `filter-search.png`                               | A search filter applied, with its "N of M" count          |
| `filter-no-match.png`                             | The empty state when no Application matches               |
| `tooltip-sync.png`                                | Tooltip on a Sync badge, opened by keyboard focus         |
| `tooltip-time.png`                                | Tooltip with the exact time behind "Last change"          |
| `detail-overview.png`                             | Application detail: source, sync policy, conditions       |
| `detail-plan-awaiting-approval.png`               | Plan tab of `podinfo`, waiting for manual approval        |
| `detail-diagnosis-degraded.png`                   | Diagnosis tab of `podinfo-broken`: the missing Secret     |
| `detail-diagnosis.png`                            | Diagnosis tab of a healthy Application                    |
| `detail-plan.png`                                 | Plan tab: the newest Revision's plan per resource         |
| `detail-history.png`                              | History tab: Revisions of this Application, newest first  |
| `detail-resources.png`                            | Resources tab: managed objects with sync and health       |
| `detail-resources-problems-only.png`              | Resources with "Only not synced or unhealthy" on          |
| `repositories.png`                                | Repositories page                                         |
| `revisions.png`                                   | Revisions page with its filters                           |
| `image-policies.png`                              | Image policies page with its empty state                  |
