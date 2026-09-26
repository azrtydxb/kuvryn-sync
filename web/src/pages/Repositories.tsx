import { usePoll } from "../api/client";
import type { RepoRow } from "../api/types";
import {
  FilterBar,
  NoMatch,
  SearchFilter,
  SegmentFilter,
  useFilters,
} from "../components/Filters";
import { StatusBadge } from "../components/StatusBadge";
import { Ago, Clip, Sha } from "../components/Tip";
import {
  filterRepos,
  isFiltered,
  presentValues,
  REPO_FILTERS,
  REPO_STATE_ORDER,
} from "../filters";
import { DASH } from "../format";
import { useShell, withNamespace } from "../layout/context";
import { PageHead, PollState } from "./common";

/** The Repositories page from "Kuvryn Sync Console.dc.html". */
export default function Repositories() {
  const { namespace } = useShell();
  const { filters, set, clear } = useFilters(REPO_FILTERS);
  const poll = usePoll<RepoRow[]>(
    withNamespace("/api/repositories", namespace),
  );
  const all = poll.data ?? [];
  const rows = filterRepos(all, filters);
  return (
    <>
      <PageHead
        eyebrow="Sources"
        title="Repositories"
        lead="Git sources resolved to an observed revision. Credentials are Secret references; values are never shown."
      />
      <PollState poll={poll} />
      {poll.data && (
        <FilterBar
          label="Filter repositories"
          shown={rows.length}
          total={all.length}
          filtered={isFiltered(filters)}
          onClear={clear}
        >
          <SearchFilter
            label="Search repositories"
            value={filters.q}
            onChange={(q) => set({ q }, { replace: true })}
          />
          <SegmentFilter
            label="State"
            value={filters.state}
            options={presentValues(
              all.map((r) => r.state),
              REPO_STATE_ORDER,
              filters.state,
            )}
            onChange={(state) => set({ state })}
          />
        </FilterBar>
      )}
      {poll.data && rows.length === 0 && all.length > 0 && (
        <NoMatch what="repositories" onClear={clear} />
      )}
      {poll.data && rows.length > 0 && (
        <div className="az-table-wrap">
          <table className="az-table">
            <thead>
              <tr>
                <th>Repository</th>
                <th>URL</th>
                <th>Ref</th>
                <th>Observed</th>
                <th>State</th>
                <th>Apps</th>
                <th>Poll</th>
                <th>Webhook</th>
                <th className="ks-right">Last fetch</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((r) => (
                <tr key={r.namespace + "/" + r.name}>
                  <td className="az-table__primary">
                    {r.name}
                    <small>{r.message}</small>
                  </td>
                  <td className="az-table__mono">
                    <Clip text={r.url} />
                  </td>
                  <td className="az-table__mono">{r.ref}</td>
                  <td className="az-table__mono">
                    <Sha sha={r.observed} />
                  </td>
                  <td>
                    <StatusBadge kind="repo" value={r.state} />
                  </td>
                  <td>{r.apps ?? DASH}</td>
                  <td className="az-table__mono">{r.poll}</td>
                  <td>{r.webhook ? "Signed" : DASH}</td>
                  <td className="ks-right">
                    <Ago iso={r.lastFetch} />
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
