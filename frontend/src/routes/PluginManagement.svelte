<script lang="ts">
  import {onDestroy} from "svelte";
  import {Events} from "@wailsio/runtime";
  import {RefreshCw, Trash2, ChevronDown, ChevronRight, FolderOpen} from "lucide-svelte";
  import {
    ListPlugins,
    EnablePlugin,
    DisablePlugin,
    ReloadPluginManual,
    UninstallPlugin,
    InstallPlugin,
    InstallPluginArchive,
    SaveRegistryCredentials,
    AddInsecureRegistry,
  } from "$api/github.com/Vilsol/klados/internal/services/pluginservice.js";
  import {BrowsePluginFile} from "$api/github.com/Vilsol/klados/internal/services/appservice.js";
  import {capabilitiesStore} from "$lib/stores/capabilities.svelte.js";
  import {ConfirmDialog} from "@klados/ui";
  import {notificationStore} from "$lib/stores/notification.svelte.js";

  const OCI_PREFIX_RE = /^oci:\/\//;

  interface ResourcePerm {
    group: string;
    version: string;
    resource: string;
    verbs: string[];
  }

  interface PermsSummary {
    resources?: ResourcePerm[];
    logs?: boolean;
    exec?: boolean;
    storage?: boolean;
    events?: boolean;
    wasi?: string[];
  }

  interface PluginInfo {
    name: string;
    version: string;
    displayName: string;
    description?: string;
    status: string;
    error?: string;
    conflictWarnings?: string[];
    dir?: string;
    permissions?: PermsSummary;
  }

  let plugins = $state<PluginInfo[]>([]);
  let expandedPerms = $state<Record<string, boolean>>({});
  let uninstallTarget = $state<string | null>(null);
  let confirmOpen = $state(false);
  let loadingActions = $state<Record<string, boolean>>({});
  let installing = $state(false);
  let registryRef = $state("");
  let registryLoading = $state(false);
  let registryError = $state("");
  let showAuthForm = $state(false);
  let authHost = $state("");
  let authUsername = $state("");
  let authPassword = $state("");
  let authInsecure = $state(false);

  async function loadPlugins() {
    try {
      const result = await ListPlugins();
      plugins = (result ?? []) as PluginInfo[];
    } catch {
      plugins = [];
    }
  }

  const unsub = Events.On("plugins:loaded", () => loadPlugins());
  onDestroy(() => unsub());

  loadPlugins();

  function statusColor(status: string) {
    switch (status) {
      case "active":
        return "text-green-500";
      case "disabled":
        return "text-muted";
      case "errored":
        return "text-red-500";
      default:
        return "text-muted";
    }
  }

  async function withLoading(name: string, fn: () => Promise<void>) {
    loadingActions[name] = true;
    try {
      await fn();
    } catch (err) {
      notificationStore.error(`Action failed for "${name}"`, err instanceof Error ? err.message : String(err));
    } finally {
      loadingActions[name] = false;
    }
  }

  async function togglePlugin(p: PluginInfo) {
    await withLoading(p.name, async () => {
      if (p.status === "disabled") {
        await EnablePlugin(p.name);
      } else {
        await DisablePlugin(p.name);
      }
      await loadPlugins();
    });
  }

  async function reloadPlugin(name: string) {
    await withLoading(name, async () => {
      await ReloadPluginManual(name);
    });
  }

  async function confirmUninstall() {
    if (!uninstallTarget) {
      return;
    }
    const name = uninstallTarget;
    uninstallTarget = null;
    await withLoading(name, async () => {
      await UninstallPlugin(name);
      await loadPlugins();
    });
  }

  async function installFromRegistry() {
    const ref = registryRef.startsWith("oci://") ? registryRef : `oci://${registryRef}`;
    registryLoading = true;
    registryError = "";
    try {
      await InstallPlugin(ref);
      registryRef = "";
      showAuthForm = false;
      notificationStore.success("Plugin installed", ref.split("/").pop() ?? ref);
      await loadPlugins();
    } catch (err: unknown) {
      if (err instanceof Error && err.message.includes("authentication required")) {
        authHost = ref.replace(OCI_PREFIX_RE, "").split("/")[0];
        showAuthForm = true;
      } else {
        registryError = err instanceof Error ? err.message : String(err);
      }
    } finally {
      registryLoading = false;
    }
  }

  async function submitCredentials() {
    const ref = registryRef.startsWith("oci://") ? registryRef : `oci://${registryRef}`;
    registryLoading = true;
    try {
      await SaveRegistryCredentials(authHost, authUsername, authPassword);
      if (authInsecure) {
        await AddInsecureRegistry(authHost);
      }
      await InstallPlugin(ref);
      showAuthForm = false;
      authUsername = "";
      authPassword = "";
      authInsecure = false;
      registryRef = "";
      notificationStore.success("Plugin installed", ref.split("/").pop() ?? ref);
      await loadPlugins();
    } catch (err: unknown) {
      if (err instanceof Error && err.message.includes("authentication required")) {
        registryError = "Credentials rejected — verify and try again";
      } else {
        registryError = err instanceof Error ? err.message : String(err);
      }
    } finally {
      registryLoading = false;
    }
  }

  let pluginFileInput = $state<HTMLInputElement>();

  // Desktop: native open dialog → install by path (like pre-split).
  // Web: hidden file input → upload the archive bytes.
  async function installPlugin() {
    if (!capabilitiesStore.nativeDialogs) {
      pluginFileInput?.click();
      return;
    }
    installing = true;
    try {
      const path = await BrowsePluginFile();
      if (!path) {
        return;
      }
      await InstallPlugin(path);
      notificationStore.success("Plugin installed", path.split("/").pop() ?? path);
    } catch (err) {
      notificationStore.error("Install failed", err instanceof Error ? err.message : String(err));
    } finally {
      installing = false;
    }
  }

  async function onPluginFileChosen(e: Event) {
    const input = e.currentTarget as HTMLInputElement;
    const file = input.files?.[0];
    if (!file) {
      return;
    }
    installing = true;
    try {
      const data = new Uint8Array(await file.arrayBuffer());
      await InstallPluginArchive(data, file.name);
      notificationStore.success("Plugin installed", file.name);
    } catch (err) {
      notificationStore.error("Install failed", err instanceof Error ? err.message : String(err));
    } finally {
      installing = false;
      input.value = "";
    }
  }
</script>

<div class="flex flex-col h-full overflow-auto">
  <div class="flex items-center gap-2 px-6 py-4 border-b border-border">
    <h1 class="text-lg font-semibold">Plugins</h1>
    {#if plugins.length > 0}
      <span class="text-xs bg-surface px-2 py-0.5 rounded-full border border-border text-muted"> {plugins.length} </span>
    {/if}
    <div class="ml-auto">
      <input
        type="file"
        accept=".gz,.tar,.oci.tar.gz,.oci.tar,application/gzip"
        class="hidden"
        bind:this={pluginFileInput}
        onchange={onPluginFileChosen}
      >
      <button
        type="button"
        onclick={installPlugin}
        disabled={installing}
        class="flex items-center gap-1.5 text-xs px-3 py-1.5 rounded border border-border hover:bg-surface-hover transition-colors disabled:opacity-50"
        title="Install plugin from .oci.tar.gz archive"
      >
        <FolderOpen size={13} />
        {installing ? 'Installing…' : 'Install Plugin'}
      </button>
    </div>
  </div>

  <div class="px-6 py-4 border-b border-border flex flex-col gap-3">
    <p class="text-sm font-medium">Install from registry</p>
    <div class="flex gap-2">
      <input
        type="text"
        bind:value={registryRef}
        placeholder="ghcr.io/owner/plugin:v1"
        disabled={registryLoading}
        class="flex-1 text-xs px-3 py-1.5 rounded border border-border bg-surface focus:outline-none focus:ring-1 focus:ring-accent disabled:opacity-50"
        onkeydown={(e) => e.key === 'Enter' && installFromRegistry()}
      >
      <button
        type="button"
        onclick={installFromRegistry}
        disabled={registryLoading || !registryRef}
        class="text-xs px-3 py-1.5 rounded border border-border hover:bg-surface-hover transition-colors disabled:opacity-50"
      >
        {registryLoading ? 'Installing…' : 'Install'}
      </button>
    </div>

    {#if registryError}
      <p class="text-xs text-red-500">{registryError}</p>
    {/if}

    {#if showAuthForm}
      <div class="flex flex-col gap-2 border border-border rounded-lg p-3 bg-surface text-xs">
        <p class="text-muted">Authentication required for <span class="text-fg font-mono">{authHost}</span></p>
        <input
          type="text"
          bind:value={authUsername}
          placeholder="Username"
          class="px-2 py-1 rounded border border-border bg-surface focus:outline-none"
        >
        <input
          type="password"
          bind:value={authPassword}
          placeholder="Password or token"
          class="px-2 py-1 rounded border border-border bg-surface focus:outline-none"
        >
        <label class="flex items-center gap-2 cursor-pointer">
          <input type="checkbox" bind:checked={authInsecure}>
          Insecure (HTTP)
        </label>
        <button
          type="button"
          onclick={submitCredentials}
          disabled={registryLoading}
          class="self-start px-3 py-1 rounded border border-border hover:bg-surface-hover transition-colors disabled:opacity-50"
        >
          Save & Retry
        </button>
      </div>
    {/if}
  </div>

  <div class="flex-1 px-6 py-4">
    {#if plugins.length === 0}
      <div class="flex flex-col items-center justify-center py-16 text-muted text-sm gap-2">
        <p>No plugins installed.</p>
        <p class="text-xs">Add plugin directories to the plugins folder to get started.</p>
      </div>
    {:else}
      <div class="flex flex-col gap-3">
        {#each plugins as plugin (plugin.name)}
          <div class="border border-border rounded-lg bg-surface overflow-hidden">
            <!-- Header row -->
            <div class="flex items-start gap-3 px-4 py-3">
              <div class="flex-1 min-w-0">
                <div class="flex items-center gap-2 flex-wrap">
                  <span class="font-medium text-sm">{plugin.displayName || plugin.name}</span>
                  <span class="text-xs text-muted">v{plugin.version}</span>
                  <span class="text-xs font-medium {statusColor(plugin.status)} capitalize"> {plugin.status} </span>
                </div>
                {#if plugin.description}
                  <p class="text-xs text-muted mt-0.5 truncate">{plugin.description}</p>
                {/if}
                {#if plugin.error && plugin.status === 'errored'}
                  <p class="text-xs text-red-500 mt-1 font-mono break-all">{plugin.error}</p>
                {/if}
                {#if plugin.conflictWarnings && plugin.conflictWarnings.length > 0}
                  <div class="mt-1 flex flex-col gap-0.5">
                    {#each plugin.conflictWarnings as warning}
                      <p class="text-xs text-yellow-500">⚠ {warning}</p>
                    {/each}
                  </div>
                {/if}
              </div>

              <!-- Actions -->
              <div class="flex items-center gap-1 shrink-0">
                <!-- Enable/Disable toggle -->
                <button
                  type="button"
                  onclick={() => togglePlugin(plugin)}
                  disabled={loadingActions[plugin.name]}
                  class="text-xs px-2 py-1 rounded border border-border hover:bg-surface-hover transition-colors disabled:opacity-50"
                  title={plugin.status === 'disabled' ? 'Enable plugin' : 'Disable plugin'}
                >
                  {plugin.status === 'disabled' ? 'Enable' : 'Disable'}
                </button>

                <!-- Reload button -->
                <button
                  type="button"
                  onclick={() => reloadPlugin(plugin.name)}
                  disabled={loadingActions[plugin.name]}
                  class="p-1.5 rounded hover:bg-surface-hover transition-colors text-muted hover:text-fg disabled:opacity-50"
                  title="Reload plugin"
                  aria-label="Reload {plugin.name}"
                >
                  <RefreshCw size={14} />
                </button>

                <!-- Uninstall button -->
                <button
                  type="button"
                  onclick={() => { uninstallTarget = plugin.name; confirmOpen = true }}
                  disabled={loadingActions[plugin.name]}
                  class="p-1.5 rounded hover:bg-surface-hover transition-colors text-muted hover:text-destructive disabled:opacity-50"
                  title="Uninstall plugin"
                  aria-label="Uninstall {plugin.name}"
                >
                  <Trash2 size={14} />
                </button>
              </div>
            </div>

            <!-- Permission summary (expandable) -->
            <div class="border-t border-border">
              <button
                type="button"
                onclick={() => (expandedPerms[plugin.name] = !expandedPerms[plugin.name])}
                class="w-full flex items-center gap-1 px-4 py-1.5 text-xs text-muted hover:bg-surface-hover transition-colors text-left"
              >
                {#if expandedPerms[plugin.name]}
                  <ChevronDown size={12} />
                {:else}
                  <ChevronRight size={12} />
                {/if}
                Permissions
              </button>
              {#if expandedPerms[plugin.name]}
                <div class="px-4 pb-3 text-xs text-muted space-y-1">
                  {#if !plugin.permissions}
                    <p class="italic text-muted/60">No permissions declared.</p>
                  {:else}
                    {@const p = plugin.permissions}
                    {#if p.resources && p.resources.length > 0}
                      <div>
                        <span class="font-medium text-fg">Resources:</span>
                        {#each p.resources as r}
                          <div class="ml-2 font-mono">{r.group}/{r.version}/{r.resource} [{r.verbs.join(', ')}]</div>
                        {/each}
                      </div>
                    {/if}
                    {#if p.logs || p.exec || p.storage || p.events}
                      <div class="flex flex-wrap gap-1 mt-0.5">
                        {#if p.logs}
                          <span class="bg-surface border border-border rounded px-1">logs</span>
                        {/if}
                        {#if p.exec}
                          <span class="bg-surface border border-border rounded px-1">exec</span>
                        {/if}
                        {#if p.storage}
                          <span class="bg-surface border border-border rounded px-1">storage</span>
                        {/if}
                        {#if p.events}
                          <span class="bg-surface border border-border rounded px-1">events</span>
                        {/if}
                      </div>
                    {/if}
                    {#if p.wasi && p.wasi.length > 0}
                      <div><span class="font-medium text-fg">WASI:</span> {p.wasi.join(', ')}</div>
                    {/if}
                  {/if}
                  {#if plugin.dir}
                    <p class="mt-1 font-mono text-muted/60 truncate" title={plugin.dir}>{plugin.dir}</p>
                  {/if}
                </div>
              {/if}
            </div>
          </div>
        {/each}
      </div>
    {/if}
  </div>
</div>

<ConfirmDialog
  bind:open={confirmOpen}
  title="Uninstall plugin"
  message={`Are you sure you want to uninstall "${uninstallTarget ?? ''}"? This will delete the plugin directory and cannot be undone.`}
  confirmLabel="Uninstall"
  onconfirm={confirmUninstall}
/>
