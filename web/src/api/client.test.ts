import { describe, expect, it, vi } from "vitest";
import { getJSON } from "./client";
describe("getJSON", () => {
  it("sends the browser to /login on 401", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(new Response("", { status: 401 })),
    );
    const loc = { href: "/apps" };
    vi.stubGlobal("location", loc);
    await expect(getJSON("/api/applications")).rejects.toThrow("unauthorized");
    expect(loc.href).toBe("/login");
  });
});
