import { useEffect } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { usePoll } from "../api/client";
import type {
  AppDetail,
  Cause,
  PlanView,
  ResourceRow,
  RevisionRow,
} from "../api/types";
import { Command } from "../components/Command";
import {
  FilterBar,
  NoMatch,
  SearchFilter,
  SelectFilter,
  SwitchFilter,
  useFilters,
} from "../components/Filters";
import { StatusBadge } from "../components/StatusBadge";
import { Ago, Clip, Digest, Sha, Tip } from "../components/Tip";
import {
  filterHistory,
  filterResources,
  HISTORY_FILTERS,
  isFiltered,
  PHASE_ORDER,
  presentValues,
  RESOURCE_FILTERS,
} from "../filters";
import { DASH, planSummary } from "../format";
import { useShell } from "../layout/context";
import { HEADER_TIP } from "../tips";
import {
  Badge,
  Button,
  EmptyState,
  Icon,
  PropertyList,
  Tabs,
} from "../ui/azrty";
import { PollState } from "./common";

const TABS = ["overview", "diagnosis", "plan", "history", "resources"] as const;
type Tab = (typeof TABS)[number];

const KIND_ICON: Record<string, string> = {
  Deployment: "layers",
  StatefulSet: "layers",
  DaemonSet: "layers",
  ReplicaSet: "copy",
  Pod: "box",
  Secret: "key-round",
  ConfigMap: "file-text",
  Service: "network",
  PersistentVolumeClaim: "hard-drive",
};

const NOT_VISIBLE = "not visible with your permissions";

/** One Application with the design's tabs. */
export default function ApplicationDetail() {
  const params = useParams();
  const navigate = useNavigate();
  const { setDetailCrumb } = useShell();
  const ns = params.ns ?? "";
  const name = params.name ?? "";
  const tab: Tab = TABS.includes(params.tab as Tab)
    ? (params.tab as Tab)
    : "overview";
  const base =
    "/api/applications/" +
    encodeURIComponent(ns) +
    "/" +
    encodeURIComponent(name);
  const detail = usePoll<AppDetail>(base);
  const history = usePoll<RevisionRow[]>(base + "/revisions");
  const resources = usePoll<ResourceRow[]>(base + "/resources");

  useEffect(() => {
    setDetailCrumb(name);
    return () => setDetailCrumb(undefined);
  }, [name, setDetailCrumb]);

  const app = detail.data;
  const setTab = (t: string) =>
    navigate(
      "/apps/" +
        encodeURIComponent(ns) +
        "/" +
        encodeURIComponent(name) +
        "/" +
        t,
      { replace: true },
    );

  return (
    <>
      <div className="ks-detail-head">
        <div>
          <Button
            size="sm"
            variant="ghost"
            icon="arrow-left"
            onClick={() => navigate("/apps")}
          >
            All applications
          </Button>
        </div>
        <div className="ks-detail-title">
          <div className="ks-head">
            <span className="az-eyebrow ks-head__eyebrow">
              Application · {ns}
            </span>
            <h1 className="ks-head__title">{name}</h1>
            {app && (
              <p className="ks-head__lead ks-head__lead--mono">
                {app.repository} · <Clip text={app.path} max={48} /> ·{" "}
                {app.render} ·{" "}
                <span className="ks-nowrap">
                  runs as {app.policy.serviceAccountName}
                </span>
              </p>
            )}
          </div>
          {app && (
            <div className="ks-badges">
              <StatusBadge kind="sync" value={app.sync} />
              <StatusBadge kind="health" value={app.health} />
            </div>
          )}
        </div>
        <Tabs
          items={[
            { id: "overview", label: "Overview" },
            {
              id: "diagnosis",
              label: "Diagnosis",
              count: app?.diagnosis.length || undefined,
            },
            { id: "plan", label: "Plan" },
            { id: "history", label: "History", count: history.data?.length },
            {
              id: "resources",
              label: "Resources",
              count: resources.data?.length,
            },
          ]}
          value={tab}
          onChange={setTab}
        />
      </div>
      <PollState poll={detail} />
      {app && tab === "overview" && <Overview app={app} />}
      {app && tab === "diagnosis" && <Diagnosis app={app} ns={ns} />}
      {app && tab === "plan" && <PlanTab app={app} ns={ns} />}
      {tab === "history" && <HistoryTab rows={history.data} />}
      {tab === "resources" && <ResourcesTab rows={resources.data} />}
    </>
  );
}

function onOff(v: boolean, off = "Off"): string {
  return v ? "On" : off;
}

function Overview({ app }: { app: AppDetail }) {
  const p = app.policy;
  return (
    <>
      <div className="ks-cards">
        <div className="az-card ks-card">
          <h3 className="ks-card__title">Source</h3>
          <PropertyList
            items={[
              { label: "Repository", value: app.source.repository, mono: true },
              {
                label: "Revision",
                value: (
                  <>
                    {app.source.revision} → <Sha sha={app.deployedRevision} />
                  </>
                ),
                mono: true,
              },
              {
                label: "Path",
                value: <Clip text={app.source.path} max={40} />,
                mono: true,
              },
              { label: "Renderer", value: app.source.render },
              {
                label: "Service account",
                value: p.serviceAccountName,
                mono: true,
              },
              { label: "Destination", value: app.destination, mono: true },
            ]}
          />
        </div>
        <div className="az-card ks-card">
          <h3 className="ks-card__title">Sync policy</h3>
          <PropertyList
            items={[
              {
                label: "Automatic sync",
                value: onOff(p.automatic, "Off · manual approval"),
              },
              { label: "Prune", value: onOff(p.prune) },
              {
                label: "Self-heal",
                value: onOff(p.selfHeal, "Off · drift reported only"),
              },
              { label: "Conflict policy", value: p.conflictPolicy, mono: true },
              { label: "On failure", value: p.failureAction },
              { label: "Deletion policy", value: p.deletionPolicy },
              { label: "Suspended", value: p.suspend ? "Yes" : "No" },
            ]}
          />
        </div>
      </div>
      <h2 className="ks-section">Conditions</h2>
      {app.conditions.length === 0 ? (
        <EmptyState
          icon="list-checks"
          title="No conditions yet"
          description="The controller has not reported this Application's status."
        />
      ) : (
        <div className="az-table-wrap">
          <table className="az-table">
            <thead>
              <tr>
                <th>Type</th>
                <th>Status</th>
                <th>Reason</th>
                <th>Message</th>
                <th className="ks-right">Last transition</th>
              </tr>
            </thead>
            <tbody>
              {app.conditions.map((c) => (
                <tr key={c.type}>
                  <td className="az-table__primary">{c.type}</td>
                  <td>
                    <StatusBadge
                      kind="condition"
                      value={c.status}
                      dot={false}
                    />
                  </td>
                  <td className="az-table__mono">{c.reason}</td>
                  <td className="ks-wrap ks-wrap--wide">{c.message}</td>
                  <td className="ks-right">
                    <Ago iso={c.lastTransitionTime} />
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

function CauseCard({ cause }: { cause: Cause }) {
  return (
    <div className="az-card ks-cause">
      <div className="ks-cause__head">
        <Badge tone="bad" icon="circle-x">
          {cause.reason}
        </Badge>
        <span className="ks-cause__resource">{cause.resource}</span>
      </div>
      <p className="ks-cause__message">{cause.message}</p>
      <div className="ks-chain">
        <span className="az-eyebrow">Chain to root cause</span>
        {cause.chain.map((n, i) => (
          <div
            key={i}
            className="ks-chain__link"
            style={{ paddingLeft: i * 22 }}
          >
            <Icon
              name={KIND_ICON[n.kind] ?? "box"}
              size={14}
              className="ks-chain__icon"
            />
            <span>
              <span className="ks-muted">{n.kind}/</span>
              {n.name}
            </span>
            {/* The root's state is the cause's reason, already shown above. */}
            {n.state !== DASH && n.state !== cause.reason && (
              <Badge tone="bad">{n.state}</Badge>
            )}
          </div>
        ))}
      </div>
    </div>
  );
}

function Diagnosis({ app, ns }: { app: AppDetail; ns: string }) {
  return (
    <>
      {app.diagnosis.length > 0 ? (
        <>
          <p className="ks-intro">
            Walked from each unhealthy managed resource down the live resource
            graph to the most specific evidence. Recorded in status.diagnosis.
          </p>
          {app.diagnosis.map((c, i) => (
            <CauseCard key={i} cause={c} />
          ))}
        </>
      ) : (
        <EmptyState
          icon="circle-check"
          title="No causes recorded"
          description="No managed resource is Degraded; status.diagnosis is empty."
        />
      )}
      <Command
        title="Same view from the CLI"
        code={
          "ksync diagnose " +
          app.name +
          " -n " +
          ns +
          "\n" +
          "ksync graph " +
          app.name +
          " -n " +
          ns +
          " -o dot | dot -Tsvg > " +
          app.name +
          ".svg"
        }
      />
    </>
  );
}

function PlanTab({ app, ns }: { app: AppDetail; ns: string }) {
  if (!app.planVisible) {
    return (
      <EmptyState
        icon="eye-off"
        title="No plan shown"
        description={"Revisions are " + NOT_VISIBLE + "."}
      />
    );
  }
  const plan: PlanView | null = app.plan;
  if (!plan) {
    return (
      <EmptyState
        icon="file-diff"
        title="No plan yet"
        description="No Revision has been planned for this Application."
      />
    );
  }
  const counts: [string, number][] = [
    ["Create", plan.summary.create],
    ["Update", plan.summary.update],
    ["Delete", plan.summary.delete],
    ["Unchanged", plan.summary.unchanged],
  ];
  const awaiting =
    app.sync === "AwaitingApproval" || plan.phase === "AwaitingApproval";
  return (
    <>
      <div className="az-card ks-plan">
        <div className="ks-plan__rev">
          <span className="az-eyebrow">Newest revision</span>
          <span className="ks-plan__name">{plan.revision}</span>
          <span className="ks-plan__digest">
            plan digest <Digest digest={plan.digest} />
          </span>
        </div>
        {counts.map(([label, value]) => (
          <div key={label} className="ks-plan__count">
            <span className="az-eyebrow">{label}</span>
            <span className="ks-plan__value">{value}</span>
          </div>
        ))}
        <StatusBadge kind="phase" value={plan.phase} />
      </div>
      <div className="az-table-wrap">
        <table className="az-table">
          <thead>
            <tr>
              <th>Action</th>
              <th>Resource</th>
              <th>Field changes</th>
              <th>Notes</th>
            </tr>
          </thead>
          <tbody>
            {plan.resources.length === 0 && (
              <tr>
                <td>
                  <StatusBadge kind="action" value="Unchanged" dot={false} />
                </td>
                <td className="az-table__mono ks-strong">
                  {plan.summary.unchanged} resources
                </td>
                <td className="ks-wrap" />
                <td className="ks-note">
                  Live state matches rendered desired state.
                </td>
              </tr>
            )}
            {plan.resources.map((r, i) => (
              <tr key={i}>
                <td>
                  <StatusBadge kind="action" value={r.action} dot={false} />
                </td>
                <td className="az-table__mono ks-strong">
                  {r.ref.kind}/{r.ref.namespace ? r.ref.namespace + "/" : ""}
                  {r.ref.name}
                </td>
                <td className="ks-wrap">
                  <div className="ks-changes">
                    {r.changes.map((f, j) => (
                      <div key={j} className="ks-change">
                        <span className="ks-muted">{f.path}</span>
                        <br />
                        {f.redacted ? (
                          "redacted"
                        ) : (
                          <>
                            {f.before || DASH} →{" "}
                            <span className="ks-change__after">
                              {f.after || DASH}
                            </span>
                          </>
                        )}
                      </div>
                    ))}
                  </div>
                </td>
                <td className="ks-note">{r.warnings.join(" ")}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {awaiting && (
        <Command
          title="Approve this exact plan digest with the CLI · recorded under your identity"
          code={
            "ksync plan " +
            app.name +
            " -n " +
            ns +
            "\n" +
            "ksync sync " +
            app.name +
            " -n " +
            ns +
            " --revision " +
            plan.revision
          }
        />
      )}
    </>
  );
}

function HistoryTab({ rows }: { rows: RevisionRow[] | undefined }) {
  const { filters, set, clear } = useFilters(HISTORY_FILTERS);
  if (!rows) return null;
  if (rows.length === 0) {
    return (
      <EmptyState
        icon="history"
        title="No revisions yet"
        description="No deployment has been attempted for this Application."
      />
    );
  }
  const shown = filterHistory(rows, filters);
  return (
    <>
      <FilterBar
        label="Filter history"
        shown={shown.length}
        total={rows.length}
        filtered={isFiltered(filters)}
        onClear={clear}
      >
        <SelectFilter
          label="Phase"
          all="All phases"
          value={filters.phase}
          options={presentValues(
            rows.map((r) => r.phase),
            PHASE_ORDER,
            filters.phase,
          )}
          onChange={(phase) => set({ phase })}
        />
      </FilterBar>
      {shown.length === 0 ? (
        <NoMatch what="revisions" onClear={clear} />
      ) : (
        <div className="az-table-wrap">
          <table className="az-table">
            <thead>
              <tr>
                <th>Revision</th>
                <th>Commit</th>
                <th>Phase</th>
                <th>
                  <Tip text={HEADER_TIP.plan} side="bottom">
                    Plan
                  </Tip>
                </th>
                <th>Approved by</th>
                <th>Attempts</th>
                <th className="ks-right">Started</th>
              </tr>
            </thead>
            <tbody>
              {shown.map((h) => (
                <tr key={h.name}>
                  <td className="az-table__mono ks-strong">{h.name}</td>
                  <td className="az-table__mono">
                    <Sha sha={h.commit} />
                  </td>
                  <td>
                    <StatusBadge kind="phase" value={h.phase} />
                  </td>
                  <td className="az-table__mono">{planSummary(h.plan)}</td>
                  <td>{h.approvedBy}</td>
                  <td>{h.attempts || DASH}</td>
                  <td className="ks-right">
                    <Ago iso={h.started} />
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

function ResourcesTab({ rows }: { rows: ResourceRow[] | undefined }) {
  const { filters, set, clear } = useFilters(RESOURCE_FILTERS);
  if (!rows) return null;
  if (rows.length === 0) {
    return (
      <EmptyState
        icon="layers"
        title="No managed resources"
        description="This Application has not applied any resources yet."
      />
    );
  }
  const shown = filterResources(rows, filters);
  return (
    <>
      <FilterBar
        label="Filter resources"
        shown={shown.length}
        total={rows.length}
        filtered={isFiltered(filters)}
        onClear={clear}
      >
        <SearchFilter
          label="Search resources"
          value={filters.q}
          onChange={(q) => set({ q }, { replace: true })}
        />
        <SelectFilter
          label="Kind"
          all="All kinds"
          value={filters.kind}
          options={presentValues(
            rows.map((r) => r.kind),
            [],
            filters.kind,
          )}
          onChange={(kind) => set({ kind })}
        />
        <SwitchFilter
          label="Only not synced or unhealthy"
          checked={filters.attention === "1"}
          onChange={(on) => set({ attention: on ? "1" : "" })}
        />
      </FilterBar>
      {shown.length === 0 ? (
        <NoMatch what="resources" onClear={clear} />
      ) : (
        <div className="az-table-wrap">
          <table className="az-table">
            <thead>
              <tr>
                <th>Kind</th>
                <th>Name</th>
                <th>API version</th>
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
              </tr>
            </thead>
            <tbody>
              {shown.map((m, i) => (
                <tr key={i}>
                  <td className="az-table__primary">{m.kind}</td>
                  <td className="az-table__mono">
                    {m.visible ? (
                      m.name
                    ) : (
                      <span className="ks-muted">
                        {m.name === DASH
                          ? NOT_VISIBLE
                          : m.name + " · " + NOT_VISIBLE}
                      </span>
                    )}
                  </td>
                  <td className="az-table__mono">{m.apiVersion}</td>
                  <td>
                    {m.sync === DASH ? (
                      DASH
                    ) : (
                      <StatusBadge kind="sync" value={m.sync} dot={false} />
                    )}
                  </td>
                  <td>
                    {m.health === DASH ? (
                      DASH
                    ) : (
                      <StatusBadge kind="health" value={m.health} />
                    )}
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
