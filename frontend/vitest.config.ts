import {defineConfig} from "vitest/config";
import {svelte} from "@sveltejs/vite-plugin-svelte";
import {svelteTesting} from "@testing-library/svelte/vite";
import path from "node:path";

export default defineConfig({
  plugins: [svelte(), svelteTesting()],
  test: {
    environment: "jsdom",
    include: ["src/**/*.test.ts"],
    setupFiles: ["src/lib/__tests__/setup.ts"],
    server: {
      deps: {
        // Ships extensionless relative imports in its ESM dist; Node can't
        // resolve them when externalized, so let Vite transform it.
        inline: ["codemirror-json-schema"],
      },
    },
  },
  resolve: {
    alias: {
      $lib: path.resolve("./src/lib"),
      $api: path.resolve("./src/api"),
      // Same alias as vite.config.ts: the Wails bridge is replaced by the
      // Connect-backed shim (tests still vi.mock this module id).
      "@wailsio/runtime": path.resolve("./src/lib/transport/wails-runtime.ts"),
    },
  },
});
