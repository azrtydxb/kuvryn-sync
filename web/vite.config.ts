import { writeFileSync } from "node:fs";
import { resolve } from "node:path";
import react from "@vitejs/plugin-react";
import type { Plugin } from "vite";
import { defineConfig } from "vitest/config";

const outDir = resolve(__dirname, "../internal/console/ui/dist");

// emptyOutDir removes the tracked .gitkeep that keeps the embed directory in
// Git; put it back after every build.
function keepGitkeep(): Plugin {
  return {
    name: "ksync-keep-gitkeep",
    closeBundle() {
      writeFileSync(resolve(outDir, ".gitkeep"), "");
    },
  };
}

export default defineConfig({
  plugins: [react(), keepGitkeep()],
  build: {
    outDir,
    emptyOutDir: true,
    // The console's CSP allows fonts and scripts only from 'self', so no
    // asset may be inlined as a data: URI.
    assetsInlineLimit: 0,
  },
  test: {
    include: ["src/**/*.test.ts"],
  },
});
