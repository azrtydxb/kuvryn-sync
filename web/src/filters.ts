// Client-side table filters. They narrow rows the page already fetched and
// live in the URL's query string, so they survive reload and Back and can be
// shared. This module is pure; components/Filters.tsx binds it to the URL.
import type {
  AppRow,
  ImagePolicyRow,
  RepoRow,
  ResourceRow,
  RevisionRow,
} from "./api/types";
import { DASH } from "./format";

/** Filter values by query-string key; an absent key is "All". */
export type Filters = Partial<Record<string, string>>;

export const APP_FILTERS = ["q", "sync", "health"] as const;
export const REVISION_FILTERS = ["q", "app", "phase"] as const;
export const REPO_FILTERS = ["q", "state"] as const;
export const POLICY_FILTERS = ["q"] as const;
export const RESOURCE_FILTERS = ["q", "kind", "attention"] as const;
export const HISTORY_FILTERS = ["phase"] as const;

/** The Sync filter value that matches every state in NOT_SYNCED_STATES. */
export const NOT_SYNCED = "NotSynced";

/** Sync states that are neither Synced, awaiting approval, nor unknown. */
export const NOT_SYNCED_STATES = [
  "OutOfSync",
  "Drifted",
  "Applying",
  "Planning",
  "Pruning",
];

/** The order states appear in filter options, from api/v1alpha1. */
export const SYNC_ORDER = [
  "Synced",
  "OutOfSync",
  "Drifted",
  "AwaitingApproval",
  "Planning",
  "Applying",
  "Pruning",
  "Unknown",
];
export const HEALTH_ORDER = [
  "Healthy",
  "Progressing",
  "Degraded",
  "Suspended",
  "Unknown",
];
export const PHASE_ORDER = [
  "Pending",
  "Planning",
  "AwaitingApproval",
  "Applying",
  "Observing",
  "Healthy",
  "Failed",
  "RollingBack",
  "RolledBack",
  "Cancelled",
];
export const REPO_STATE_ORDER = ["Ready", "Failed", "Unknown"];

/** Reads the named filters from a query string, leaving out empty ones. */
export function readFilters(
  params: URLSearchParams,
  keys: readonly string[],
): Filters {
  const out: Filters = {};
  for (const k of keys) {
    const v = params.get(k);
    if (v) out[k] = v;
  }
  return out;
}

/**
 * Returns params with the named filters set to next: keys next leaves out
 * or empties are removed, and every other parameter is kept.
 */
export function writeFilters(
  params: URLSearchParams,
  keys: readonly string[],
  next: Filters,
): URLSearchParams {
  const out = new URLSearchParams(params);
  for (const k of keys) out.delete(k);
  for (const k of keys) {
    const v = next[k];
    if (v) out.set(k, v);
  }
  return out;
}

/** Whether any filter is set. */
export function isFiltered(f: Filters): boolean {
  return Object.values(f).some(Boolean);
}

/** Whether every whitespace-separated term of q is in one of fields. */
export function matchesQuery(
  q: string | undefined,
  ...fields: (string | undefined)[]
): boolean {
  const terms = (q ?? "").toLowerCase().split(/\s+/).filter(Boolean);
  const hay = fields.map((f) => (f ?? "").toLowerCase());
  return terms.every((t) => hay.some((h) => h.includes(t)));
}

/**
 * The distinct values present, in order's order and then alphabetically,
 * without "—"; selected is kept so a shared link's filter stays visible.
 */
export function presentValues(
  values: string[],
  order: readonly string[],
  selected?: string,
): string[] {
  const set = new Set(values.filter((v) => v && v !== DASH));
  if (selected) set.add(selected);
  const rank = (v: string) => {
    const i = order.indexOf(v);
    return i < 0 ? order.length : i;
  };
  return [...set].sort((a, b) => rank(a) - rank(b) || a.localeCompare(b));
}

/** Whether a sync state matches the Sync filter. */
export function matchesSync(filter: string | undefined, sync: string): boolean {
  if (!filter) return true;
  if (filter === NOT_SYNCED) return NOT_SYNCED_STATES.includes(sync);
  return sync === filter;
}

const is = (filter: string | undefined, value: string) =>
  !filter || filter === value;

export function filterApps(rows: AppRow[], f: Filters): AppRow[] {
  return rows.filter(
    (a) =>
      matchesQuery(f.q, a.name, a.path, a.repository) &&
      matchesSync(f.sync, a.sync) &&
      is(f.health, a.health),
  );
}

export function filterRevisions(
  rows: RevisionRow[],
  f: Filters,
): RevisionRow[] {
  return rows.filter(
    (r) =>
      matchesQuery(f.q, r.name, r.application, r.commit, r.approvedBy) &&
      is(f.app, r.application) &&
      is(f.phase, r.phase),
  );
}

export function filterHistory(rows: RevisionRow[], f: Filters): RevisionRow[] {
  return rows.filter((r) => is(f.phase, r.phase));
}

export function filterRepos(rows: RepoRow[], f: Filters): RepoRow[] {
  return rows.filter(
    (r) =>
      matchesQuery(f.q, r.name, r.url, r.ref, r.message) &&
      is(f.state, r.state),
  );
}

export function filterPolicies(
  rows: ImagePolicyRow[],
  f: Filters,
): ImagePolicyRow[] {
  return rows.filter((p) =>
    matchesQuery(f.q, p.name, p.image, p.rule, p.latest),
  );
}

/** Whether a managed resource is known to be not synced or not healthy. */
export function needsAttention(r: ResourceRow): boolean {
  return (
    (r.sync !== DASH && r.sync !== "Synced") ||
    (r.health !== DASH && r.health !== "Healthy")
  );
}

export function filterResources(
  rows: ResourceRow[],
  f: Filters,
): ResourceRow[] {
  return rows.filter(
    (r) =>
      matchesQuery(f.q, r.kind, r.name, r.apiVersion) &&
      is(f.kind, r.kind) &&
      (!f.attention || needsAttention(r)),
  );
}
