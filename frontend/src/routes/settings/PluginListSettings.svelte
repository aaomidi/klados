<script lang="ts">
  import {onMount} from "svelte";
  import {push} from "svelte-spa-router";
  import {ListPlugins, GetPluginSettingsSchema} from "$api/github.com/Vilsol/klados/internal/services/pluginservice.js";
  import {getLogger} from "$lib/logger";

  const log = getLogger("settings");

  interface PluginEntry {
    name: string;
    displayName: string;
    hasSettings: boolean;
  }

  let plugins = $state<PluginEntry[]>([]);
  let loading = $state<boolean>(true);

  onMount(() => {
    (async () => {
      try {
        const list = await ListPlugins();
        const entries: PluginEntry[] = [];
        for (const p of list ?? []) {
          let hasSettings = false;
          try {
            // biome-ignore lint/performance/noAwaitInLoops: sequential schema fetch with per-plugin error isolation
            const schema = await GetPluginSettingsSchema(p.name);
            hasSettings = Boolean(schema) && schema !== "" && schema !== "{}";
          } catch {
            // no settings schema
          }
          entries.push({
            name: p.name,
            displayName: p.displayName || p.name,
            hasSettings,
          });
        }
        plugins = entries;
      } catch (e) {
        log.error("Failed to load plugins", {error: String(e)});
      } finally {
        loading = false;
      }
    })();
  });

  let pluginsWithSettings = $derived(plugins.filter((p) => p.hasSettings));
</script>

<div class="max-w-2xl space-y-6">
  <h2 class="text-base font-medium text-fg mb-4">Plugin Settings</h2>

  {#if loading}
    <p class="text-sm text-muted-foreground">Loading plugins...</p>
  {:else if pluginsWithSettings.length === 0}
    <p class="text-sm text-muted-foreground">No plugins with configurable settings found.</p>
  {:else}
    <div class="border border-border rounded overflow-hidden divide-y divide-border">
      {#each pluginsWithSettings as plugin}
        <button
          type="button"
          class="w-full flex items-center justify-between px-4 py-3 text-left hover:bg-surface-hover transition-colors"
          onclick={() => push(`/settings/plugins/${encodeURIComponent(plugin.name)}`)}
        >
          <span class="text-sm text-fg">{plugin.displayName}</span>
          <svg aria-hidden="true" class="w-4 h-4 text-muted-foreground" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" />
          </svg>
        </button>
      {/each}
    </div>
  {/if}
</div>
