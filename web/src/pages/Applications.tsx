import { useNavigate } from "react-router-dom";
import { usePoll } from "../api/client";
import type { AppDetail, AppRow } from "../api/types";
import { StatusBadge } from "../components/StatusBadge";
import { ago, shortSha } from "../format";
import { useShell, withNamespace } from "../layout/context";
import { Alert, Button, StatCard } from "../ui/azrty";
import { PageHead, PollState } from "./common";

const NOT_SYNCED = ["OutOfSync", "Drifted", "Applying", "Planning", "Pruning"];
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

/** The Applications list from "Kuvryn Sync Console.dc.html". */
export default function Applications() {
  const navigate = useNavigate();
  const { namespace } = useShell();
  const poll = usePoll<AppRow[]>(withNamespace("/api/applications", namespace));
  const apps = poll.data ?? [];
  const degraded = apps.find((a) => a.health === "Degraded");
  const diagnosis = usePoll<AppDetail>(
    degraded
      ? "/api/applications/" + degraded.namespace + "/" + degraded.name
      : null,
  );
  const cause = diagnosis.data?.diagnosis[0];
  const open = (a: AppRow, tab?: string) =>
    navigate(
      "/apps/" +
        encodeURIComponent(a.namespace) +
        "/" +
        encodeURIComponent(a.name) +
        (tab ? "/" + tab : ""),
    );

  const repos = new Set(apps.map((a) => a.repository)).size;
  const healthy = count(apps, (a) => a.health === "Healthy");
  const others = ["Degraded", "Progressing", "Suspended"]
    .map((h) => [count(apps, (a) => a.health === h), h.toLowerCase()] as const)
    .filter(([n]) => n > 0)
    .map(([n, h]) => n + " " + h);
  const notSynced = apps.filter((a) => NOT_SYNCED.includes(a.sync));
  const awaiting = apps.filter((a) => a.sync === "AwaitingApproval");

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
          <StatCard
            label="Applications"
            value={apps.length}
            icon="boxes"
            sub={
              "from " + repos + (repos === 1 ? " repository" : " repositories")
            }
          />
          <StatCard
            label="Healthy"
            value={healthy}
            unit={"/ " + apps.length}
            icon="circle-check"
            sub={others.join(" · ") || "every application"}
          />
          <StatCard
            label="Not synced"
            value={notSynced.length}
            icon="git-compare"
            sub={
              [...new Set(notSynced.map((a) => LABEL[a.sync]))].join(" · ") ||
              "all in sync"
            }
          />
          <StatCard
            label="Awaiting approval"
            value={awaiting.length}
            icon="user-check"
            sub={awaiting.map((a) => a.name).join(" · ") || "no plan waiting"}
          />
        </div>
      )}
      {poll.data && (
        <div className="az-table-wrap">
          <table className="az-table">
            <thead>
              <tr>
                <th>Application</th>
                <th>Destination</th>
                <th>Repository</th>
                <th>Commit</th>
                <th>Sync</th>
                <th>Health</th>
                <th className="ks-right">Last reconcile</th>
              </tr>
            </thead>
            <tbody>
              {apps.map((a) => (
                <tr
                  key={a.namespace + "/" + a.name}
                  className="az-table__row--click"
                  tabIndex={0}
                  onClick={() => open(a)}
                  onKeyDown={(e) => e.key === "Enter" && open(a)}
                >
                  <td className="az-table__primary">
                    {a.name}
                    <small>
                      {a.path} · {a.render}
                    </small>
                  </td>
                  <td className="az-table__mono">{a.destination}</td>
                  <td className="az-table__mono">{a.repository}</td>
                  <td className="az-table__mono">{shortSha(a.commit)}</td>
                  <td>
                    <StatusBadge kind="sync" value={a.sync} />
                  </td>
                  <td>
                    <StatusBadge kind="health" value={a.health} />
                  </td>
                  <td className="ks-right">{ago(a.lastReconcile)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </>
  );
}
