import { stateTip } from "../tips";
import { Badge, type Tone } from "../ui/azrty";
import { Tip } from "./Tip";

// Tone maps from "Kuvryn Sync Console.dc.html" (SYNC_T, HEALTH_T, PHASE_T,
// ACT_T), extended to every state the API reports.
export const SYNC_T: Record<string, Tone> = {
  Synced: "good",
  OutOfSync: "warn",
  Drifted: "warn",
  AwaitingApproval: "info",
  Applying: "neutral",
  Planning: "neutral",
  Pruning: "neutral",
  Unknown: "outline",
};
export const HEALTH_T: Record<string, Tone> = {
  Healthy: "good",
  Degraded: "bad",
  Progressing: "neutral",
  Suspended: "outline",
  Unknown: "outline",
};
export const PHASE_T: Record<string, Tone> = {
  Healthy: "good",
  Failed: "bad",
  AwaitingApproval: "info",
  Applying: "neutral",
  Observing: "neutral",
  Pending: "neutral",
  Planning: "neutral",
  RollingBack: "warn",
  RolledBack: "warn",
  Cancelled: "outline",
};
export const ACT_T: Record<string, Tone> = {
  Create: "good",
  Update: "info",
  Delete: "bad",
  Unchanged: "outline",
};

export type BadgeKind =
  "sync" | "health" | "phase" | "action" | "condition" | "repo";

function toneOf(kind: BadgeKind, value: string): Tone {
  switch (kind) {
    case "sync":
      return SYNC_T[value] ?? "outline";
    case "health":
      return HEALTH_T[value] ?? "outline";
    case "phase":
      return PHASE_T[value] ?? "neutral";
    case "action":
      return ACT_T[value] ?? "outline";
    case "condition":
      return value === "True" ? "good" : value === "False" ? "bad" : "outline";
    case "repo":
      return value === "Ready"
        ? "good"
        : value === "Failed"
          ? "bad"
          : "outline";
  }
}

/**
 * A state badge in the design's tone for its kind. Sync and health badges
 * explain their state in a tooltip.
 */
export function StatusBadge({
  kind,
  value,
  dot = true,
}: {
  kind: BadgeKind;
  value: string;
  dot?: boolean;
}) {
  const badge = (
    <Badge tone={toneOf(kind, value)} dot={dot}>
      {value}
    </Badge>
  );
  const tip = stateTip(kind, value);
  return tip ? <Tip text={tip}>{badge}</Tip> : badge;
}
