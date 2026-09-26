import { describe, expect, it } from "vitest";
import { HEALTH_ORDER, SYNC_ORDER } from "./filters";
import { absolute } from "./format";
import { HEALTH_TIP, stateTip, SYNC_TIP } from "./tips";

describe("TestStateTooltips", () => {
  it("explains every sync and health state in one sentence", () => {
    for (const [states, tips] of [
      [SYNC_ORDER, SYNC_TIP],
      [HEALTH_ORDER, HEALTH_TIP],
    ] as const) {
      for (const s of states) {
        expect(tips[s], s).toMatch(new RegExp("^" + s + ": [^.]+$"));
      }
    }
    expect(stateTip("sync", "OutOfSync")).toBe(
      "OutOfSync: Git differs from the cluster",
    );
    expect(stateTip("health", "Nonsense")).toBeUndefined();
    expect(stateTip("phase", "Healthy")).toBeUndefined();
  });

  it("shows an absolute local time", () => {
    const iso = "2026-09-26T08:05:09Z";
    const d = new Date(iso);
    const p = (n: number) => String(n).padStart(2, "0");
    expect(absolute(iso)).toMatch(
      new RegExp(
        "^" +
          d.getFullYear() +
          "-" +
          p(d.getMonth() + 1) +
          "-" +
          p(d.getDate()) +
          " " +
          p(d.getHours()) +
          ":" +
          p(d.getMinutes()) +
          ":09( \\S+)?$",
      ),
    );
    expect(absolute("—")).toBe("—");
    expect(absolute("not a time")).toBe("—");
  });
});
