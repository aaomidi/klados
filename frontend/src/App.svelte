<script lang="ts">
  import Router from "svelte-spa-router";
  import {routes} from "./routes/routes";
  import Layout from "$lib/components/Layout.svelte";
  import {Notification} from "@klados/ui";
  import CommandPalette from "$lib/components/CommandPalette.svelte";
  import {onMount} from "svelte";
  import {clusterStore} from "$lib/stores/cluster.svelte";
  import {sessionStore} from "$lib/stores/session.svelte";
  import {setTheme} from "$lib/theme.svelte";
  import {descriptorRegistry} from "$lib/registry/index";
  import "$lib/navTiming";
  import {shortcutStore} from "$lib/stores/shortcuts.svelte";
  import {Events} from "@wailsio/runtime";
  import {LogFrontend, GetSession, SaveUIState} from "$api/github.com/Vilsol/klados/internal/services/appservice.js";
  import type {TabState} from "$api/github.com/Vilsol/klados/internal/session/models.js";
  import PanelWindow from "$lib/components/PanelWindow.svelte";
  import StackTrace from "stacktrace-js";
  import CreateResourceDialog from "$lib/components/CreateResourceDialog.svelte";
  import {createResourceStore} from "$lib/stores/createResource.svelte";
  import ApplyManifestDialog from "$lib/components/ApplyManifestDialog.svelte";
  import {applyManifestStore} from "$lib/stores/applyManifest.svelte";
  import {preferencesStore} from "$lib/stores/preferences.svelte";
  import ShortcutHelpDialog from "$lib/components/ShortcutHelpDialog.svelte";
  import {shortcutActions} from "$lib/stores/shortcutActions.svelte";
  import {push} from "svelte-spa-router";
  import VolumeBrowserDialog from "$lib/components/VolumeBrowserDialog.svelte";
  import VolumeBrowserCollisionDialog from "$lib/components/VolumeBrowserCollisionDialog.svelte";
  import OrphanCleanupToast from "$lib/components/OrphanCleanupToast.svelte";
  import {volumeBrowserStore} from "$lib/stores/volumeBrowser.svelte";
  import {notificationStore} from "$lib/stores/notification.svelte";
  import {focusedPVC} from "$lib/utils/focusedPVC";

  const panelId = new URLSearchParams(window.location.search).get("panel");
  let paletteOpen = $state(false);
  let shortcutsHelpOpen = $state(false);

  function logToBackend(level: string, message: string, detail: string) {
    LogFrontend(level, message, detail).catch(() => {
      /* empty */
    });
  }

  async function logError(msg: string, err: Error | undefined) {
    let stack = "";
    if (err) {
      try {
        const frames = await StackTrace.fromError(err);
        stack = frames
          .map((f) => `  at ${f.getFunctionName() ?? "<anonymous>"} (${f.getFileName()}:${f.getLineNumber()}:${f.getColumnNumber()})`)
          .join("\n");
      } catch {
        stack = err.stack ?? "";
      }
    }
    logToBackend("error", msg, stack);
  }

  function argsToLog(args: unknown[]): [string, Error | undefined] {
    const err = args.find((a) => a instanceof Error) as Error | undefined;
    const msg = args.map((a) => (a instanceof Error ? a.message : String(a))).join(" ");
    return [msg, err];
  }

  onMount(() => {
    // biome-ignore lint/suspicious/noConsole: intentional console override
    const origDebug = console.debug.bind(console);
    // biome-ignore lint/suspicious/noConsole: intentional console override
    const origError = console.error.bind(console);
    // biome-ignore lint/suspicious/noConsole: intentional console override
    const origWarn = console.warn.bind(console);
    console.debug = (...args) => {
      const [m] = argsToLog(args);
      logToBackend("debug", m, "");
    };
    console.error = (...args) => {
      origError(...args);
      const [m, e] = argsToLog(args);
      logError(m, e);
    };
    console.warn = (...args) => {
      origWarn(...args);
      const [m, e] = argsToLog(args);
      logToBackend("warn", m, e?.stack ?? "");
    };
    window.onerror = (_msg, _src, _line, _col, err) => {
      logError(String(_msg), err);
    };
    window.onunhandledrejection = (e) => {
      const err = e.reason instanceof Error ? e.reason : undefined;
      const msg = err ? err.message : String(e.reason);
      logError(`Unhandled rejection: ${msg}`, err);
    };

    let unsubPlugins: (() => void) | undefined;
    let handler: ((e: KeyboardEvent) => void) | undefined;

    (async () => {
      try {
        const sess = await GetSession();
        if (sess) {
          const tabs = (sess.openTabs ?? []).map((t: TabState) => ({
            clusterContext: t.clusterContext ?? "",
            gvr: t.gvr ?? "",
            namespace: t.namespace ?? "",
            name: t.name ?? "",
            scrollPosition: t.scrollPosition ?? 0,
          }));
          sessionStore.restore(tabs, sess.activeTab ?? 0, sess.sidebarCollapsed ?? false, sess.terminalFontSize || undefined, sess.sidebarWidth || undefined);
        }
      } catch {
        // Session restore not available
      }

      await clusterStore.loadContexts();
      preferencesStore.subscribe();
      await preferencesStore.load(clusterStore.activeContext ?? "");
      await descriptorRegistry.load();

      unsubPlugins = Events.On("plugins:loaded", () => {
        descriptorRegistry.reloadPlugins();
      });

      shortcutStore.register({
        id: "command-palette",
        keys: "Control+k",
        description: "Open command palette",
        modes: ["normal", "editor"],
        category: "General",
        action: () => {
          paletteOpen = true;
        },
      });

      shortcutStore.register({
        id: "shortcut-help",
        keys: "?",
        description: "Show keyboard shortcuts",
        category: "General",
        action: () => {
          shortcutsHelpOpen = true;
        },
      });

      shortcutStore.register({
        id: "focus-search",
        keys: "/",
        description: "Focus search / filter bar",
        category: "General",
        action: () => {
          shortcutActions.focusSearch++;
        },
      });

      shortcutStore.register({
        id: "focus-search-ctrl",
        keys: "Control+f",
        description: "Focus search / filter bar",
        category: "General",
        hidden: true,
        action: () => {
          shortcutActions.focusSearch++;
        },
      });

      shortcutStore.register({
        id: "escape",
        keys: "Escape",
        description: "Close dialog / go back",
        modes: ["normal", "editor"],
        category: "General",
        action: () => {
          if (shortcutsHelpOpen) {
            shortcutsHelpOpen = false;
          } else if (paletteOpen) {
            paletteOpen = false;
          } else if (createResourceStore.open) {
            createResourceStore.close();
          } else if (applyManifestStore.open) {
            applyManifestStore.close();
          } else if (location.hash.match(/^#\/c\/[^/]+\/[^/]+\/[^/]+\/[^/]+$/)) {
            // On detail page — navigate back to resource list
            const parts = location.hash.slice(2).split("/");
            push(`/c/${parts[1]}/${parts[2]}`);
          }
        },
      });

      shortcutStore.register({
        id: "confirm-dialog",
        keys: "Control+Enter",
        description: "Confirm / submit dialog",
        modes: ["normal", "editor"],
        category: "General",
        action: () => {
          shortcutActions.confirmDialog++;
        },
      });

      shortcutStore.register({
        id: "toggle-sidebar",
        keys: "Control+b",
        description: "Toggle sidebar",
        category: "Navigation",
        action: () => {
          sessionStore.toggleSidebar();
        },
      });

      shortcutStore.register({
        id: "create-resource",
        keys: "Control+Shift+N",
        description: "Create new resource",
        category: "Resources",
        action: () => {
          createResourceStore.openDialog();
        },
      });

      shortcutStore.register({
        id: "apply-manifest",
        keys: "Control+Shift+A",
        description: "Apply manifest",
        category: "Resources",
        action: () => {
          applyManifestStore.openDialog();
        },
      });

      shortcutStore.register({
        id: "browse-volume",
        keys: "",
        description: "Browse volume of selected PVC",
        category: "Resources",
        action: () => {
          if (!clusterStore.canMutate()) {
            return;
          }
          const pvc = focusedPVC();
          if (!pvc) {
            notificationStore.push("Select a PVC to browse its volume", "info");
            return;
          }
          void volumeBrowserStore.spawn(pvc.contextName, pvc.namespace, pvc.name);
        },
      });

      shortcutStore.register({
        id: "refresh",
        keys: "F5",
        description: "Refresh resource list",
        category: "Resources",
        action: () => {
          shortcutActions.refreshList++;
        },
      });

      handler = (e: KeyboardEvent) => shortcutStore.dispatch(e);
      window.addEventListener("keydown", handler);

      // WebKitGTK doesn't wire Ctrl+Z/Y to undo/redo for native inputs
      window.addEventListener("keydown", (e: KeyboardEvent) => {
        const el = document.activeElement;
        if (!(el instanceof HTMLInputElement || el instanceof HTMLTextAreaElement)) {
          return;
        }
        if (e.ctrlKey && !e.altKey && !e.metaKey) {
          if (e.key === "z" && !e.shiftKey) {
            e.preventDefault();
            document.execCommand("undo");
          } else if ((e.key === "z" && e.shiftKey) || e.key === "y") {
            e.preventDefault();
            document.execCommand("redo");
          }
        }
      });
    })();

    return () => {
      if (handler) {
        window.removeEventListener("keydown", handler);
      }
      unsubPlugins?.();
      preferencesStore.destroy();
    };
  });

  // Reload preferences when active cluster changes
  $effect(() => {
    const ctx = clusterStore.activeContext;
    preferencesStore.load(ctx ?? "");
  });

  $effect(() => {
    const theme = preferencesStore.prefs.theme;
    if (theme) {
      setTheme(theme as "light" | "dark" | "system");
    }
  });

  $effect(() => {
    const color = preferencesStore.prefs.accentColor;
    if (color) {
      document.documentElement.style.setProperty("--color-accent", color);
    } else {
      document.documentElement.style.removeProperty("--color-accent");
    }
  });

  $effect(() => {
    const size = preferencesStore.prefs.fontSize;
    if (size > 0) {
      document.documentElement.style.setProperty("font-size", `${size}px`);
    } else {
      document.documentElement.style.removeProperty("font-size");
    }
  });

  // Persist UI state whenever tabs/sidebar/fontSize change.
  // Debounced so high-frequency changes (sidebar resize drag) don't flood Wails IPC.
  let saveUITimer: ReturnType<typeof setTimeout> | null = null;
  $effect(() => {
    const tabs = sessionStore.tabs;
    const activeTab = sessionStore.activeTabIndex;
    const sidebarCollapsed = sessionStore.sidebarCollapsed;
    const terminalFontSize = sessionStore.terminalFontSize;
    const sidebarWidth = sessionStore.sidebarWidth;
    if (saveUITimer !== null) {
      clearTimeout(saveUITimer);
    }
    saveUITimer = setTimeout(() => {
      saveUITimer = null;
      SaveUIState(
        tabs.map((t) => ({
          clusterContext: t.clusterContext,
          gvr: t.gvr,
          namespace: t.namespace,
          name: t.name,
          scrollPosition: t.scrollPosition ?? 0,
        })),
        activeTab,
        sidebarCollapsed,
        terminalFontSize,
        sidebarWidth,
      ).catch(() => {
        /* empty */
      });
    }, 150);
  });
</script>

{#if panelId}
  <PanelWindow {panelId} />
{:else}
  <Layout> <Router {routes} /> </Layout>
  <CommandPalette bind:open={paletteOpen} />
  <CreateResourceDialog
    bind:open={createResourceStore.open}
    ctxName={clusterStore.activeContext ?? ''}
    gvr={createResourceStore.gvr}
    onsuccess={createResourceStore.onsuccess}
  />
  <ApplyManifestDialog bind:open={applyManifestStore.open} ctxName={clusterStore.activeContext ?? ''} />
  <ShortcutHelpDialog bind:open={shortcutsHelpOpen} />
  {#if volumeBrowserStore.dialog}
    {@const d = volumeBrowserStore.dialog}
    <VolumeBrowserDialog
      open={true}
      namespace={d.namespace}
      pvcName={d.pvcName}
      initial={d.initial}
      onsubmit={(overrides) => d.resolve(overrides)}
      oncancel={() => d.resolve(null)}
    />
  {/if}
  {#if volumeBrowserStore.collision}
    {@const c = volumeBrowserStore.collision}
    <VolumeBrowserCollisionDialog
      open={true}
      existingPodName={c.existingPodName}
      pvcName={c.pvcName}
      onattach={() => c.resolve('attach')}
      onreplace={() => c.resolve('replace')}
      oncancel={() => c.resolve('cancel')}
    />
  {/if}
  <OrphanCleanupToast />
{/if}
<Notification />
