import { useNavigate } from "react-router-dom";
import { usePoll } from "../api/client";
import type { RevisionRow } from "../api/types";
import { StatusBadge } from "../components/StatusBadge";
import { ago, planSummary, shortSha } from "../format";
import { useShell, withNamespace } from "../layout/context";
import { PageHead, PollState } from "./common";

/** The Revisions page from "Kuvryn Sync Console.dc.html". */
export default function Revisions() {
  const navigate = useNavigate();
  const { namespace } = useShell();
  const poll = usePoll<RevisionRow[]>(
    withNamespace("/api/revisions", namespace),
  );
  const open = (r: RevisionRow) =>
    navigate(
      "/apps/" +
        encodeURIComponent(r.namespace) +
        "/" +
        encodeURIComponent(r.application) +
        "/history",
    );
  return (
    <>
      <PageHead
        eyebrow="Audit"
        title="Revisions"
        lead="One record per deployment attempt, newest first. Bounded and redacted."
      />
      <PollState poll={poll} />
      {poll.data && (
        <div className="az-table-wrap">
          <table className="az-table">
            <thead>
              <tr>
                <th>Revision</th>
                <th>Application</th>
                <th>Commit</th>
                <th>Phase</th>
                <th>Plan</th>
                <th>Approved by</th>
                <th className="ks-right">Started</th>
              </tr>
            </thead>
            <tbody>
              {poll.data.map((r) => (
                <tr
                  key={r.namespace + "/" + r.name}
                  className="az-table__row--click"
                  tabIndex={0}
                  onClick={() => open(r)}
                  onKeyDown={(e) => e.key === "Enter" && open(r)}
                >
                  <td className="az-table__mono ks-strong">{r.name}</td>
                  <td>{r.application}</td>
                  <td className="az-table__mono">{shortSha(r.commit)}</td>
                  <td>
                    <StatusBadge kind="phase" value={r.phase} />
                  </td>
                  <td className="az-table__mono">{planSummary(r.plan)}</td>
                  <td>{r.approvedBy}</td>
                  <td className="ks-right">{ago(r.started)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </>
  );
}
