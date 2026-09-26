import type { KeyboardEvent, ReactNode } from "react";
import { Link, useNavigate } from "react-router-dom";
import { usePoll } from "../api/client";
import type { AppDetail, AppRow } from "../api/types";
import {
  FilterBar,
  NoMatch,
  SearchFilter,
  SegmentFilter,
  SelectFilter,
  useFilters,
} from "../components/Filters";
import { StatusBadge } from "../components/StatusBadge";
import { Ago, Clip, Sha, Tip } from "../components/Tip";
import {
  APP_FILTERS,
  filterApps,
  HEALTH_ORDER,
  isFiltered,
  NOT_SYNCED,
  NOT_SYNCED_STATES,
  presentValues,
  SYNC_ORDER,
} from "../filters";
import { useShell, withNamespace } from "../layout/context";
import { HEADER_TIP, STAT_TIP } from "../tips";
import { Alert, Button, StatCard } from "../ui/azrty";
import { PageHead, PollState } from "./common";

const LABEL: Record<string, string> = {
  OutOfSync: "out of sync",
  Drifted: "drifted",
  Applying: "applying",
  Planning: "planning",
  Pruning: "pruning",
};

function count<T>(items: T[], pred: (t: T) => boolean): number {
  return items.filter(pred).length;
}

/**
 * A stat card that sets a filter: a keyboard-reachable toggle button, with
 * what it counts in a tooltip.
 */
function FilterCard({
  tip,
  pressed,
  onToggle,
  children,
}: {
  tip: string;
  pressed: boolean;
  onToggle: () => void;
  children: ReactNode;
}) {
  const onKeyDown = (e: KeyboardEvent) => {
    if (e.key === "Enter" || e.key === " ") {
      e.preventDefault();
      onToggle();
    }
  };
  return (
    <Tip text={tip} asChild className="ks-stat-tip">
      <div
        role="button"
        tabIndex={0}
        aria-pressed={pressed}
        className="ks-statbtn"
        onClick={onToggle}
        onKeyDown={onKeyDown}
      >
        {children}
      </div>
    </Tip>
  );
}

/** The Applications list from "Kuvryn Sync Console.dc.html". */
export default function Applications() {
  const navigate = useNavigate();
  const { namespace } = useShell();
  const { filters, set, clear } = useFilters(APP_FILTERS);
  const poll = usePoll<AppRow[]>(withNamespace("/api/applications", namespace));
  const apps = poll.data ?? [];
  const rows = filterApps(apps, filters);
  const degraded = apps.find((a) => a.health === "Degraded");
  const diagnosis = usePoll<AppDetail>(
    degraded
      ? "/api/applications/" + degraded.namespace + "/" + degraded.name
      : null,
  );
  const cause = diagnosis.data?.diagnosis[0];
  const href = (a: AppRow, tab?: string) =>
    "/apps/" +
    encodeURIComponent(a.namespace) +
    "/" +
    encodeURIComponent(a.name) +
    (tab ? "/" + tab : "");
  const open = (a: AppRow, tab?: string) => navigate(href(a, tab));

  const repos = new Set(apps.map((a) => a.repository)).size;
  const healthy = count(apps, (a) => a.health === "Healthy");
  const others = ["Degraded", "Progressing", "Suspended"]
    .map((h) => [count(apps, (a) => a.health === h), h.toLowerCase()] as const)
    .filter(([n]) => n > 0)
    .map(([n, h]) => n + " " + h);
  const notSynced = apps.filter((a) => NOT_SYNCED_STATES.includes(a.sync));
  const awaiting = apps.filter((a) => a.sync === "AwaitingApproval");
  const toggle = (key: "sync" | "health", value: string) =>
    set({ [key]: filters[key] === value ? "" : value });

  const syncOptions = [
    { value: NOT_SYNCED, label: "Not synced" },
    ...presentValues(
      apps.map((a) => a.sync),
      SYNC_ORDER,
      filters.sync === NOT_SYNCED ? undefined : filters.sync,
    ).map((s) => ({ value: s, label: s })),
  ];

  return (
    <>
      <PageHead
        eyebrow="GitOps"
        title="Applications"
        lead="Sync and health are reported separately. Changes are made in Git or with the ksync CLI."
      />
      <PollState poll={poll} />
      {degraded && (
        <Alert
          tone="bad"
          title={degraded.name + " is Degraded"}
          action={
            <Button
              size="sm"
              variant="secondary"
              icon="stethoscope"
              onClick={() => open(degraded, "diagnosis")}
            >
              View diagnosis
            </Button>
          }
        >
          {cause
            ? cause.reason + ": " + cause.message
            : "No cause is recorded yet."}
        </Alert>
      )}
      {poll.data && (
        <div className="ks-stats">
          <Tip text={STAT_TIP.applications} className="ks-stat-tip">
            <StatCard
              label="Applications"
              value={apps.length}
              icon="boxes"
              sub={
                "from " +
                repos +
                (repos === 1 ? " repository" : " repositories")
              }
            />
          </Tip>
          <FilterCard
            tip={STAT_TIP.healthy}
            pressed={filters.health === "Healthy"}
            onToggle={() => toggle("health", "Healthy")}
          >
            <StatCard
              label="Healthy"
              value={healthy}
              unit={"/ " + apps.length}
              icon="circle-check"
              sub={others.join(" · ") || "every application"}
            />
          </FilterCard>
          <FilterCard
            tip={STAT_TIP.notSynced}
            pressed={filters.sync === NOT_SYNCED}
            onToggle={() => toggle("sync", NOT_SYNCED)}
          >
            <StatCard
              label="Not synced"
              value={notSynced.length}
              icon="git-compare"
              sub={
                [...new Set(notSynced.map((a) => LABEL[a.sync]))].join(" · ") ||
                "all in sync"
              }
            />
          </FilterCard>
          <FilterCard
            tip={STAT_TIP.awaiting}
            pressed={filters.sync === "AwaitingApproval"}
            onToggle={() => toggle("sync", "AwaitingApproval")}
          >
            <StatCard
              label="Awaiting approval"
              value={awaiting.length}
              icon="user-check"
              sub={awaiting.map((a) => a.name).join(" · ") || "no plan waiting"}
            />
          </FilterCard>
        </div>
      )}
      {poll.data && (
        <FilterBar
          label="Filter applications"
          shown={rows.length}
          total={apps.length}
          filtered={isFiltered(filters)}
          onClear={clear}
        >
          <SearchFilter
            label="Search applications"
            value={filters.q}
            onChange={(q) => set({ q }, { replace: true })}
          />
          <SelectFilter
            label="Sync"
            all="All sync states"
            value={filters.sync}
            options={syncOptions}
            onChange={(sync) => set({ sync })}
          />
          <SegmentFilter
            label="Health"
            value={filters.health}
            options={presentValues(
              apps.map((a) => a.health),
              HEALTH_ORDER,
              filters.health,
            )}
            onChange={(health) => set({ health })}
          />
        </FilterBar>
      )}
      {poll.data && rows.length === 0 && apps.length > 0 && (
        <NoMatch what="applications" onClear={clear} />
      )}
      {poll.data && rows.length > 0 && (
        <div className="az-table-wrap">
          <table className="az-table">
            <thead>
              <tr>
                <th>Application</th>
                <th>Destination</th>
                <th>Repository</th>
                <th>Commit</th>
                <th>
                  <Tip text={HEADER_TIP.sync} side="bottom">
                    Sync
                  </Tip>
                </th>
                <th>
                  <Tip text={HEADER_TIP.health} side="bottom">
                    Health
                  </Tip>
                </th>
                <th className="ks-right">
                  <Tip text={HEADER_TIP.lastChange} side="bottom" align="end">
                    Last change
                  </Tip>
                </th>
              </tr>
            </thead>
            <tbody>
              {rows.map((a) => (
                <tr
                  key={a.namespace + "/" + a.name}
                  className="az-table__row--click"
                  onClick={() => open(a)}
                >
                  <td className="az-table__primary">
                    <Link
                      className="ks-rowlink"
                      to={href(a)}
                      onClick={(e) => e.stopPropagation()}
                    >
                      {a.name}
                    </Link>
                    <small>
                      <Clip text={a.path} max={32} align="start" /> · {a.render}
                    </small>
                  </td>
                  <td className="az-table__mono">{a.destination}</td>
                  <td className="az-table__mono">{a.repository}</td>
                  <td className="az-table__mono">
                    <Sha sha={a.commit} />
                  </td>
                  <td>
                    <StatusBadge kind="sync" value={a.sync} />
                  </td>
                  <td>
                    <StatusBadge kind="health" value={a.health} />
                  </td>
                  <td className="ks-right">
                    <Ago iso={a.lastChange} />
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </>
  );
}
