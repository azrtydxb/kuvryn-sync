import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

// The emblems come from the Kuvryn Sync design project. A placeholder or a
// copy truncated by the design tool's 256 KiB read limit must not ship.
describe("TestEmblemsAreTheDesignArtwork", () => {
  for (const variant of ["dark", "light"]) {
    it(`ships the complete ${variant} emblem`, () => {
      const png = readFileSync(
        new URL(`./assets/kuvryn-sync-emblem-${variant}.png`, import.meta.url),
      );
      expect(png.subarray(1, 4).toString("latin1")).toBe("PNG");
      expect(png.readUInt32BE(16)).toBe(640);
      expect(png.readUInt32BE(20)).toBe(640);
      expect(
        png.subarray(png.length - 8, png.length - 4).toString("latin1"),
      ).toBe("IEND");
      expect(png.length).toBeGreaterThan(200_000);
    });
  }
});
