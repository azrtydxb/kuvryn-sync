# Decisions

No pending decisions.

## Scout rollout blocked by before-first-apply field ownership (2026-09-26)

- Unblock kw and fix Kuvryn Sync: drop the stale before-first-apply managedFields entry on the Scout Deployment, `ksync sync` the held commit; then release 0.7.1 where a conflict only with before-first-apply is adopted instead of failing (recommended)
- Only unblock kw now; decide the Kuvryn Sync behaviour later
- Neither: set Scout's Application to conflictPolicy adopt instead

Decided: unblock kw and fix Kuvryn Sync (0.7.1).
