import { usePoll } from "../api/client";
import type { ImagePolicyRow } from "../api/types";
import {
  FilterBar,
  NoMatch,
  NothingYet,
  SearchFilter,
  useFilters,
} from "../components/Filters";
import { Ago, Clip, Digest } from "../components/Tip";
import { filterPolicies, isFiltered, POLICY_FILTERS } from "../filters";
import { useShell, withNamespace } from "../layout/context";
import { PageHead, PollState } from "./common";

/** The Image policies page from "Kuvryn Sync Console.dc.html". */
export default function ImagePolicies() {
  const { namespace } = useShell();
  const { filters, set, clear } = useFilters(POLICY_FILTERS);
  const poll = usePoll<ImagePolicyRow[]>(
    withNamespace("/api/imagepolicies", namespace),
  );
  const all = poll.data ?? [];
  const rows = filterPolicies(all, filters);
  return (
    <>
      <PageHead
        eyebrow="Automation"
        title="Image policies"
        lead="Registry scans that commit new image digests back to Git."
      />
      <PollState poll={poll} />
      {poll.data && all.length > 0 && (
        <FilterBar
          label="Filter image policies"
          shown={rows.length}
          total={all.length}
          filtered={isFiltered(filters)}
          onClear={clear}
        >
          <SearchFilter
            label="Search image policies"
            value={filters.q}
            onChange={(q) => set({ q }, { replace: true })}
          />
        </FilterBar>
      )}
      {poll.data && all.length === 0 && (
        <NothingYet
          icon="scan-search"
          what="image policies"
          description="Image policies scan a registry and commit new image tags back to Git."
        />
      )}
      {poll.data && rows.length === 0 && all.length > 0 && (
        <NoMatch what="image policies" onClear={clear} />
      )}
      {poll.data && rows.length > 0 && (
        <div className="az-table-wrap">
          <table className="az-table">
            <thead>
              <tr>
                <th>Policy</th>
                <th>Image</th>
                <th>Rule</th>
                <th>Latest</th>
                <th className="ks-right">Last scan</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((p) => (
                <tr key={p.namespace + "/" + p.name}>
                  <td className="az-table__primary">{p.name}</td>
                  <td className="az-table__mono">
                    <Clip text={p.image} />
                  </td>
                  <td className="az-table__mono">{p.rule}</td>
                  <td className="az-table__mono">
                    {p.latest}
                    <small>
                      <Digest digest={p.digest} />
                    </small>
                  </td>
                  <td className="ks-right">
                    <Ago iso={p.lastScan} />
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
