import { usePoll } from "../api/client";
import type { ImagePolicyRow } from "../api/types";
import { ago, shortDigest } from "../format";
import { useShell, withNamespace } from "../layout/context";
import { PageHead, PollState } from "./common";

/** The Image policies page from "Kuvryn Sync Console.dc.html". */
export default function ImagePolicies() {
  const { namespace } = useShell();
  const poll = usePoll<ImagePolicyRow[]>(
    withNamespace("/api/imagepolicies", namespace),
  );
  return (
    <>
      <PageHead
        eyebrow="Automation"
        title="Image policies"
        lead="Registry scans that commit new image digests back to Git."
      />
      <PollState poll={poll} />
      {poll.data && (
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
              {poll.data.map((p) => (
                <tr key={p.namespace + "/" + p.name}>
                  <td className="az-table__primary">{p.name}</td>
                  <td className="az-table__mono">{p.image}</td>
                  <td className="az-table__mono">{p.rule}</td>
                  <td className="az-table__mono">
                    {p.latest}
                    <small>{shortDigest(p.digest)}</small>
                  </td>
                  <td className="ks-right">{ago(p.lastScan)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </>
  );
}
