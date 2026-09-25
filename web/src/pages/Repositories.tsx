import { usePoll } from "../api/client";
import type { RepoRow } from "../api/types";
import { StatusBadge } from "../components/StatusBadge";
import { ago, DASH, shortSha } from "../format";
import { useShell, withNamespace } from "../layout/context";
import { PageHead, PollState } from "./common";

/** The Repositories page from "Kuvryn Sync Console.dc.html". */
export default function Repositories() {
  const { namespace } = useShell();
  const poll = usePoll<RepoRow[]>(
    withNamespace("/api/repositories", namespace),
  );
  return (
    <>
      <PageHead
        eyebrow="Sources"
        title="Repositories"
        lead="Git sources resolved to an observed revision. Credentials are Secret references; values are never shown."
      />
      <PollState poll={poll} />
      {poll.data && (
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
              {poll.data.map((r) => (
                <tr key={r.namespace + "/" + r.name}>
                  <td className="az-table__primary">
                    {r.name}
                    <small>{r.message}</small>
                  </td>
                  <td className="az-table__mono">{r.url}</td>
                  <td className="az-table__mono">{r.ref}</td>
                  <td className="az-table__mono">{shortSha(r.observed)}</td>
                  <td>
                    <StatusBadge kind="repo" value={r.state} />
                  </td>
                  <td>{r.apps ?? DASH}</td>
                  <td className="az-table__mono">{r.poll}</td>
                  <td>{r.webhook ? "Signed" : DASH}</td>
                  <td className="ks-right">{ago(r.lastFetch)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </>
  );
}
