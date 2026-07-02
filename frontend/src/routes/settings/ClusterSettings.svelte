<script lang="ts">
  import {onMount} from "svelte";
  import {push} from "svelte-spa-router";
  import {GetClusterPrefs, SetClusterPrefs} from "$api/github.com/Vilsol/klados/internal/services/configservice.js";
  import {ClusterPrefs, MetricsConfig} from "$api/github.com/Vilsol/klados/internal/config/models.js";
  import type {VolumeBrowserConfig as VBConfig} from "$lib/stores/preferences.svelte";
  import VolumeBrowserSettings from "./VolumeBrowserSettings.svelte";
  import {Disconnect, GetClusterInfo, RemoveKubeconfigPath} from "$api/github.com/Vilsol/klados/internal/services/clusterservice.js";
  import type {ClusterInfo} from "$api/github.com/Vilsol/klados/internal/services/models.js";
  import {ConnectionStatus} from "$api/github.com/Vilsol/klados/internal/cluster/models.js";
  import {ConfirmDialog} from "@klados/ui";
  import {clusterStore} from "$lib/stores/cluster.svelte.js";

  interface Props {
    ctxName: string;
  }

  let {ctxName}: Props = $props();

  let displayName = $state<string>("");
  let accentColor = $state<string>("");
  let readOnlyOverride = $state<boolean>(false);
  let readOnlyValue = $state<boolean>(false);
  let compactOverride = $state<boolean>(false);
  let compactValue = $state<boolean>(false);
  let favoriteNamespaces = $state<string[]>([]);
  let prometheusUrl = $state<string>("");
  let volumeBrowserOverride = $state<boolean>(false);
  let volumeBrowser = $state<VBConfig>({});
  let newNamespace = $state<string>("");
  let info = $state<ClusterInfo | null>(null);
  let forgetConfirmOpen = $state(false);

  onMount(() => {
    (async () => {
      const prefs = await GetClusterPrefs(ctxName);
      if (prefs) {
        displayName = prefs.displayName ?? "";
        accentColor = prefs.accentColor ?? "";
        readOnlyOverride = prefs.readOnly != null;
        readOnlyValue = prefs.readOnly ?? false;
        compactOverride = prefs.compactRows != null;
        compactValue = prefs.compactRows ?? false;
        favoriteNamespaces = prefs.favoriteNamespaces ?? [];
        prometheusUrl = prefs.metrics?.prometheusUrl ?? "";
        volumeBrowserOverride = prefs.volumeBrowser != null;
        volumeBrowser = (prefs.volumeBrowser as VBConfig | undefined) ?? {};
      }
      try {
        info = await GetClusterInfo(ctxName);
      } catch (e) {
        console.warn("GetClusterInfo failed", e);
      }
    })();
  });

  const statusLabels: Record<number, string> = {
    [ConnectionStatus.StatusDisconnected]: "disconnected",
    [ConnectionStatus.StatusConnecting]: "connecting",
    [ConnectionStatus.StatusConnected]: "connected",
    [ConnectionStatus.StatusError]: "error",
  };

  function statusLabel(s: ConnectionStatus): string {
    return statusLabels[s as number] ?? "unknown";
  }

  const canForget = $derived(Boolean(info && info.context.sourcePath && !info.context.isDefault));
  const isConnected = $derived(info?.context.status === ConnectionStatus.StatusConnected);

  function save() {
    SetClusterPrefs(ctxName, {
      displayName: displayName || undefined,
      accentColor: accentColor || undefined,
      readOnly: readOnlyOverride ? readOnlyValue : undefined,
      compactRows: compactOverride ? compactValue : undefined,
      favoriteNamespaces: favoriteNamespaces.length > 0 ? favoriteNamespaces : undefined,
      metrics: prometheusUrl ? new MetricsConfig({prometheusUrl}) : undefined,
      volumeBrowser: volumeBrowserOverride ? (volumeBrowser as never) : undefined,
    } as ClusterPrefs);
  }

  function toggleVolumeBrowserOverride(enabled: boolean) {
    volumeBrowserOverride = enabled;
    if (!enabled) {
      volumeBrowser = {};
    }
    save();
  }

  function saveVolumeBrowser(next: VBConfig) {
    volumeBrowser = next;
    save();
  }

  function setDisplayName(value: string) {
    displayName = value;
    save();
  }

  function setAccent(value: string) {
    accentColor = value;
    save();
  }

  function toggleReadOnlyOverride(enabled: boolean) {
    readOnlyOverride = enabled;
    if (!enabled) {
      readOnlyValue = false;
    }
    save();
  }

  function setReadOnly(value: boolean) {
    readOnlyValue = value;
    save();
  }

  function toggleCompactOverride(enabled: boolean) {
    compactOverride = enabled;
    if (!enabled) {
      compactValue = false;
    }
    save();
  }

  function setCompact(value: boolean) {
    compactValue = value;
    save();
  }

  function addNamespace() {
    const ns = newNamespace.trim();
    if (ns && !favoriteNamespaces.includes(ns)) {
      favoriteNamespaces = [...favoriteNamespaces, ns];
      newNamespace = "";
      save();
    }
  }

  function removeNamespace(ns: string) {
    favoriteNamespaces = favoriteNamespaces.filter((n) => n !== ns);
    save();
  }
</script>

<div class="max-w-2xl space-y-8">
  <h2 class="text-base font-medium text-fg mb-4">Cluster: {ctxName}</h2>

  {#if info}
    <section class="space-y-2">
      <h3 class="text-sm font-semibold text-fg">Cluster Info</h3>
      <dl class="grid grid-cols-[140px_1fr] gap-y-1 text-sm">
        <dt class="text-muted">Context</dt><dd class="text-fg">{info.context.name}</dd>
        <dt class="text-muted">Cluster</dt><dd class="text-fg">{info.context.cluster || "—"}</dd>
        <dt class="text-muted">User</dt><dd class="text-fg">{info.context.user || "—"}</dd>
        <dt class="text-muted">Default namespace</dt><dd class="text-fg">{info.context.namespace || "—"}</dd>
        <dt class="text-muted">Server URL</dt><dd class="text-fg break-all">{info.serverUrl || "—"}</dd>
        {#if info.context.serverVersion}
          <dt class="text-muted">Server version</dt><dd class="text-fg">{info.context.serverVersion}</dd>
        {/if}
        <dt class="text-muted">Kubeconfig</dt>
        <dd class="text-fg break-all">
          {info.context.sourcePath || "—"}
          {#if info.context.isDefault}<span class="ml-1 text-xs text-muted">(default)</span>{/if}
        </dd>
        <dt class="text-muted">Status</dt><dd class="text-fg">{statusLabel(info.context.status)}</dd>
        <dt class="text-muted">metrics-server</dt><dd class="text-fg">{info.metricsServer}</dd>
        <dt class="text-muted">Prometheus</dt>
        <dd class="text-fg break-all">
          {#if info.prometheusUrl}
            {info.prometheusUrl}
            <span class="ml-1 text-xs text-muted">({info.prometheusSource})</span>
          {:else}
            not detected
          {/if}
        </dd>
      </dl>
    </section>
  {/if}

  <div>
    <label class="block text-sm font-medium text-fg mb-1"
      >Display Name
      <input
        type="text"
        value={displayName}
        oninput={(e) => setDisplayName((e.target as HTMLInputElement).value)}
        placeholder={ctxName}
        class="w-full px-3 py-1.5 rounded border border-border bg-surface text-fg text-sm"
      >
    </label>
  </div>

  <div>
    <label for="accent-color" class="block text-sm font-medium text-fg mb-1">Accent Color</label>
    <div class="flex items-center gap-3">
      <input
        id="accent-color"
        type="color"
        value={accentColor || '#6366f1'}
        oninput={(e) => setAccent((e.target as HTMLInputElement).value)}
        class="w-8 h-8 rounded cursor-pointer border border-border"
      >
      {#if accentColor}
        <button type="button" class="text-sm text-muted-foreground hover:text-fg underline" onclick={() => setAccent('')}>Reset</button>
      {/if}
    </div>
  </div>

  <div>
    <span class="block text-sm font-medium text-fg mb-2">Read-Only</span>
    <div class="space-y-2">
      <label class="flex items-center gap-2 cursor-pointer">
        <input
          type="checkbox"
          checked={readOnlyOverride}
          onchange={(e) => toggleReadOnlyOverride((e.target as HTMLInputElement).checked)}
          class="accent-accent"
        >
        <span class="text-sm text-fg">Override global default</span>
      </label>
      {#if readOnlyOverride}
        <label class="flex items-center gap-2 cursor-pointer ml-6">
          <input
            type="checkbox"
            checked={readOnlyValue}
            onchange={(e) => setReadOnly((e.target as HTMLInputElement).checked)}
            class="accent-accent"
          >
          <span class="text-sm text-fg">Enable read-only mode for this cluster</span>
        </label>
      {:else}
        <p class="text-sm text-muted-foreground ml-6">Using global default</p>
      {/if}
    </div>
  </div>

  <div>
    <span class="block text-sm font-medium text-fg mb-2">Compact Rows</span>
    <div class="space-y-2">
      <label class="flex items-center gap-2 cursor-pointer">
        <input
          type="checkbox"
          checked={compactOverride}
          onchange={(e) => toggleCompactOverride((e.target as HTMLInputElement).checked)}
          class="accent-accent"
        >
        <span class="text-sm text-fg">Override global default</span>
      </label>
      {#if compactOverride}
        <label class="flex items-center gap-2 cursor-pointer ml-6">
          <input
            type="checkbox"
            checked={compactValue}
            onchange={(e) => setCompact((e.target as HTMLInputElement).checked)}
            class="accent-accent"
          >
          <span class="text-sm text-fg">Enable compact rows for this cluster</span>
        </label>
      {:else}
        <p class="text-sm text-muted-foreground ml-6">Using global default</p>
      {/if}
    </div>
  </div>

  <div>
    <span class="block text-sm font-medium text-fg mb-2">Favorite Namespaces</span>
    <div class="flex gap-2 mb-2">
      <input
        type="text"
        bind:value={newNamespace}
        placeholder="Namespace name"
        class="flex-1 px-3 py-1.5 rounded border border-border bg-surface text-fg text-sm"
        onkeydown={(e) => e.key === 'Enter' && addNamespace()}
      >
      <button type="button" class="px-3 py-1.5 rounded bg-accent text-accent-foreground text-sm hover:opacity-90" onclick={addNamespace}>
        Add
      </button>
    </div>
    {#if favoriteNamespaces.length > 0}
      <div class="flex flex-wrap gap-2">
        {#each favoriteNamespaces as ns}
          <span class="inline-flex items-center gap-1 px-2 py-0.5 rounded bg-surface border border-border text-sm text-fg">
            {ns}
            <button type="button" class="text-muted-foreground hover:text-fg ml-1" onclick={() => removeNamespace(ns)}>&times;</button>
          </span>
        {/each}
      </div>
    {:else}
      <p class="text-sm text-muted-foreground">No favorite namespaces configured.</p>
    {/if}
  </div>

  <section class="space-y-2">
    <h3 class="text-sm font-semibold text-fg">Metrics</h3>
    <label class="block text-sm font-medium text-fg mb-1">
      Prometheus URL
      <input
        type="text"
        value={prometheusUrl}
        oninput={(e) => { prometheusUrl = (e.target as HTMLInputElement).value; save(); }}
        placeholder={info?.prometheusUrl && info.prometheusSource === "detected" ? info.prometheusUrl : "https://prometheus.example/api/v1"}
        class="w-full px-3 py-1.5 rounded border border-border bg-surface text-fg text-sm"
      >
    </label>
    <p class="text-xs text-muted">
      {#if info?.prometheusUrl}
        Effective: {info.prometheusUrl} <span class="text-muted">({info.prometheusSource})</span>
      {:else}
        No Prometheus endpoint detected or configured.
      {/if}
    </p>
  </section>

  <section class="space-y-2">
    <h3 class="text-sm font-semibold text-fg">Volume Browser</h3>
    <label class="flex items-center gap-2 cursor-pointer">
      <input
        type="checkbox"
        checked={volumeBrowserOverride}
        onchange={(e) => toggleVolumeBrowserOverride((e.target as HTMLInputElement).checked)}
        class="accent-accent"
      >
      <span class="text-sm text-fg">Override global defaults for this cluster</span>
    </label>
    {#if volumeBrowserOverride}
      <div class="ml-6 mt-2">
        <VolumeBrowserSettings bind:value={volumeBrowser} onchange={saveVolumeBrowser} />
      </div>
    {:else}
      <p class="text-sm text-muted-foreground ml-6">Using global defaults</p>
    {/if}
  </section>

  <section class="space-y-3 pt-6 border-t border-destructive/30">
    <h3 class="text-sm font-semibold text-destructive">Actions</h3>

    <div class="flex flex-col gap-2">
      <button
        type="button"
        disabled={!isConnected}
        onclick={async () => {
          try {
            await Disconnect(ctxName);
            info = await GetClusterInfo(ctxName);
          } catch (e) {
            console.warn("disconnect failed", e);
          }
        }}
        class="self-start px-3 py-1.5 text-sm rounded border border-border hover:bg-surface-hover disabled:opacity-40 disabled:cursor-not-allowed"
      >
        Disconnect
      </button>

      {#if canForget}
        <button
          type="button"
          onclick={() => forgetConfirmOpen = true}
          class="self-start px-3 py-1.5 text-sm rounded border border-destructive/50 text-destructive hover:bg-destructive/10"
        >
          Forget cluster
        </button>
      {/if}
    </div>
  </section>
</div>

<ConfirmDialog
  bind:open={forgetConfirmOpen}
  title="Forget cluster"
  message="This will remove the kubeconfig file path from Klados. The cluster will no longer appear in the list."
  confirmLabel="Forget"
  onconfirm={async () => {
    if (!info) return;
    try {
      await RemoveKubeconfigPath(info.context.sourcePath);
      forgetConfirmOpen = false;
      await clusterStore.loadContexts();
      push("/");
    } catch (e) {
      console.warn("forget failed", e);
    }
  }}
/>
