import {defineConfig} from "vite";
import type {Plugin, ResolvedConfig, ViteDevServer} from "vite";
import {svelte} from "@sveltejs/vite-plugin-svelte";
import tailwindcss from "@tailwindcss/vite";
import path from "node:path";
import type {IncomingMessage, ServerResponse} from "node:http";
import process from "node:process";

// Public URL used in the import map — both dev and prod serve Svelte here.
const SHARED_URL = "/plugin-shared/svelte-runtime.js";
// Source file Vite transforms in dev to produce the shared runtime content.
const RUNTIME_SRC = "/src/plugin-shared/svelte-runtime.ts";

const svelteSharedRuntime = (): Plugin => {
  let isBuild = false;
  let runtimePath = "";

  return {
    name: "svelte-shared-runtime",
    enforce: "pre" as const,

    configResolved(config: ResolvedConfig) {
      isBuild = config.command === "build";
      runtimePath = path.resolve(config.root, "src/plugin-shared/svelte-runtime.ts");
    },

    resolveId(id: string, importer: string | undefined) {
      // biome-ignore lint/style/noProcessEnv: build config legitimately needs process.env
      if (!isBuild || process.env.VITEST) {
        return;
      }
      if (importer === runtimePath) {
        return; // let runtime file import real svelte
      }
      if (id === "svelte" || id === "svelte/internal/client" || id === "svelte/internal") {
        return runtimePath;
      }
    },

    // Dev only: serve the stable public URL by transforming the re-export source
    // file through Vite's pipeline. Vite rewrites bare 'svelte/internal/client'
    // to its pre-bundled dep URL (/.vite/deps/...) — the same URL the host's
    // compiled components reference — so both share one browser-cached module.
    configureServer(server: ViteDevServer) {
      server.middlewares.use(async (req: IncomingMessage, res: ServerResponse, next: () => void) => {
        if (req.url === SHARED_URL) {
          const result = await server.transformRequest(RUNTIME_SRC);
          if (result?.code) {
            res.setHeader("Content-Type", "application/javascript; charset=utf-8");
            res.setHeader("Cache-Control", "no-cache");
            res.end(result.code);
            return;
          }
        }
        next();
      });
    },
  };
};

export default defineConfig({
  plugins: [svelte(), tailwindcss(), svelteSharedRuntime()],
  resolve: {
    alias: {
      $lib: path.resolve("./src/lib"),
      // Connect-backed service facades + DTO models. Lives OUTSIDE
      // frontend/bindings so `wails3 generate bindings` can never clobber it.
      $api: path.resolve("./src/api"),
      // The Wails desktop bridge is gone; Events/Browser/System/Create are
      // served by the Connect-backed shim for app code, models, and tests.
      "@wailsio/runtime": path.resolve("./src/lib/transport/wails-runtime.ts"),
    },
  },
  build: {
    sourcemap: true,
    minify: false,
    rollupOptions: {
      // Add the runtime as an explicit entry so the bundler keeps original export
      // names (no single-letter aliasing). Entry chunks always preserve their
      // public API; internal-only chunks get mangled names.
      input: {
        index: path.resolve("./index.html"),
        "plugin-shared/svelte-runtime": path.resolve("./src/plugin-shared/svelte-runtime.ts"),
      },
      preserveEntrySignatures: "exports-only",
      output: {
        entryFileNames(chunk) {
          if (chunk.name === "plugin-shared/svelte-runtime") {
            return "[name].js";
          }
          return "assets/[name]-[hash].js";
        },
        chunkFileNames: "assets/[name]-[hash].js",
      },
    },
  },
});
