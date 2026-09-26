# ksync CLI screenshots

Real `ksync` v0.6.4 sessions against the kw cluster: the output is exactly
what the commands printed, and each `.png` has its `.txt` transcript next to
it for copying into docs. The demo namespace `ksync-demo` ran two
Applications from the public podinfo chart: `podinfo` with manual approval,
and `podinfo-broken`, whose Deployment reads a Secret that does not exist.
`kuvryn-scout` is a production Application on kw with automatic sync.

| File                 | Shows                                                                                                   |
| -------------------- | ------------------------------------------------------------------------------------------------------- |
| `01-version`         | `ksync version`                                                                                         |
| `02-help`            | `ksync help`: every command                                                                             |
| `03-install`         | `ksync install`: the release's install commands                                                         |
| `04-repos`           | `ksync repos`, `ksync repo get`                                                                         |
| `05-apps`            | `ksync apps` in a namespace                                                                             |
| `06-plan-awaiting`   | `ksync get` and `ksync plan` for a Revision waiting for approval, ending with the approve command       |
| `07-approve`         | `ksync sync --revision`: approving exactly that plan                                                    |
| `08-deployed`        | `ksync get`, `ksync history` after the rollout                                                          |
| `09-upgrade-plan`    | The plan for an upgrade to podinfo 6.15.0: image, labels, chart                                         |
| `10-upgrade-approve` | `ksync approve` (alias of `sync`) for the upgrade                                                       |
| `11-history`         | `ksync history` newest first, `ksync revision`                                                          |
| `12-rollback`        | `ksync rollback` on a manual-approval Application: one command approves and deploys the chosen Revision |
| `13-after-rollback`  | `ksync get`, `history`, `plan` after the rollback: the rolled-back commit is held                       |
| `14-suspend-resume`  | `ksync suspend`, `get`, `resume`                                                                        |
| `15-diagnose`        | `ksync diagnose`: the missing Secret as the root cause, with the chain to it                            |
| `16-graph`           | `ksync graph`: the live resource graph as a tree                                                        |
| `17-graph-dot`       | `ksync graph -o dot` for Graphviz                                                                       |
| `18-scout`           | Read commands on a production Application                                                               |
| `19-scout-plan`      | A plan with nothing to change                                                                           |
