// One short sentence per state, shown in badge tooltips. The states come
// from api/v1alpha1 (SyncState, HealthState) and internal/console/views.go.

export const SYNC_TIP: Record<string, string> = {
  Synced: "Synced: the cluster matches Git",
  OutOfSync: "OutOfSync: Git differs from the cluster",
  Drifted: "Drifted: the cluster was changed outside Git",
  AwaitingApproval: "AwaitingApproval: a plan waits for approval",
  Planning: "Planning: working out what Git would change",
  Applying: "Applying: changes from Git are being applied",
  Pruning: "Pruning: removing objects deleted from Git",
  Unknown: "Unknown: no sync state reported yet",
};

export const HEALTH_TIP: Record<string, string> = {
  Healthy: "Healthy: every managed resource is ready",
  Progressing: "Progressing: resources are still rolling out",
  Degraded: "Degraded: a managed resource has failed",
  Suspended: "Suspended: reconciliation is paused",
  Unknown: "Unknown: no health reported yet",
};

/** The tooltip for a sync or health state; undefined for anything else. */
export function stateTip(kind: string, value: string): string | undefined {
  if (kind === "sync") return SYNC_TIP[value];
  if (kind === "health") return HEALTH_TIP[value];
  return undefined;
}

export const HEADER_TIP = {
  sync: "Sync: whether the cluster matches Git. Health is separate: an app can match Git and still fail",
  health:
    "Health: whether the running resources work, whatever Git says. Sync is separate",
  plan: "Plan: +N created, ~N updated, −N deleted",
  lastChange:
    "Last change: the latest deploy started or finished, or condition change",
};

export const STAT_TIP = {
  applications: "Applications you can read in this scope",
  healthy: "Applications whose health is Healthy. Select to filter",
  notSynced:
    "Applications out of sync, drifted, or mid-sync (planning, applying, pruning). Select to filter",
  awaiting:
    "Applications whose plan waits for manual approval. Select to filter",
};

export const LIVE_TIP = "Refreshes every 10 seconds";
export const READ_ONLY_TIP =
  "The console never changes the cluster: changes are made in Git or with the ksync CLI";
