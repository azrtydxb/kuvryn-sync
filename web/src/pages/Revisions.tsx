import { Link, useNavigate } from "react-router-dom";
import { usePoll } from "../api/client";
import type { RevisionRow } from "../api/types";
import {
  FilterBar,
  NoMatch,
  SearchFilter,
  SelectFilter,
  useFilters,
} from "../components/Filters";
import { StatusBadge } from "../components/StatusBadge";
import { Ago, Sha, Tip } from "../components/Tip";
import {
  filterRevisions,
  isFiltered,
  PHASE_ORDER,
  presentValues,
  REVISION_FILTERS,
} from "../filters";
import { planSummary } from "../format";
import { useShell, withNamespace } from "../layout/context";
import { HEADER_TIP } from "../tips";
import { PageHead, PollState } from "./common";

/** The Revisions page from "Kuvryn Sync Console.dc.html". */
export default function Revisions() {
  const navigate = useNavigate();
  const { namespace } = useShell();
  const { filters, set, clear } = useFilters(REVISION_FILTERS);
  const poll = usePoll<RevisionRow[]>(
    withNamespace("/api/revisions", namespace),
  );
  const all = poll.data ?? [];
  const rows = filterRevisions(all, filters);
  const href = (r: RevisionRow) =>
    "/apps/" +
    encodeURIComponent(r.namespace) +
    "/" +
    encodeURIComponent(r.application) +
    "/history";
  const open = (r: RevisionRow) => navigate(href(r));
  return (
    <>
      <PageHead
        eyebrow="Audit"
        title="Revisions"
        lead="One record per deployment attempt, newest first. Bounded and redacted."
      />
      <PollState poll={poll} />
      {poll.data && (
        <FilterBar
          label="Filter revisions"
          shown={rows.length}
          total={all.length}
          filtered={isFiltered(filters)}
          onClear={clear}
        >
          <SearchFilter
            label="Search revisions"
            value={filters.q}
            onChange={(q) => set({ q }, { replace: true })}
          />
          <SelectFilter
            label="Application"
            all="All applications"
            value={filters.app}
            options={presentValues(
              all.map((r) => r.application),
              [],
              filters.app,
            )}
            onChange={(app) => set({ app })}
          />
          <SelectFilter
            label="Phase"
            all="All phases"
            value={filters.phase}
            options={presentValues(
              all.map((r) => r.phase),
              PHASE_ORDER,
              filters.phase,
            )}
            onChange={(phase) => set({ phase })}
          />
        </FilterBar>
      )}
      {poll.data && rows.length === 0 && all.length > 0 && (
        <NoMatch what="revisions" onClear={clear} />
      )}
      {poll.data && rows.length > 0 && (
        <div className="az-table-wrap">
          <table className="az-table">
            <thead>
              <tr>
                <th>Revision</th>
                <th>Application</th>
                <th>Commit</th>
                <th>Phase</th>
                <th>
                  <Tip text={HEADER_TIP.plan} side="bottom">
                    Plan
                  </Tip>
                </th>
                <th>Approved by</th>
                <th className="ks-right">Started</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((r) => (
                <tr
                  key={r.namespace + "/" + r.name}
                  className="az-table__row--click"
                  onClick={() => open(r)}
                >
                  <td className="az-table__mono ks-strong">
                    <Link
                      className="ks-rowlink"
                      to={href(r)}
                      onClick={(e) => e.stopPropagation()}
                    >
                      {r.name}
                    </Link>
                  </td>
                  <td>{r.application}</td>
                  <td className="az-table__mono">
                    <Sha sha={r.commit} />
                  </td>
                  <td>
                    <StatusBadge kind="phase" value={r.phase} />
                  </td>
                  <td className="az-table__mono">{planSummary(r.plan)}</td>
                  <td>{r.approvedBy}</td>
                  <td className="ks-right">
                    <Ago iso={r.started} />
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
