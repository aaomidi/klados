<script lang="ts">
  import {push} from "svelte-spa-router";
  import {ListPlugins, GetPluginSettingsSchema} from "$api/github.com/Vilsol/klados/internal/services/pluginservice.js";

  interface Props {
    active: string;
  }

  let {active}: Props = $props();

  const mainSections = [
    {id: "general", label: "General"},
    {id: "appearance", label: "Appearance"},
    {id: "clusters", label: "Clusters"},
    {id: "keybindings", label: "Keybindings"},
    {id: "filters", label: "Filters"},
    {id: "columns", label: "Columns"},
  ];

  let plugins = $state<Array<{name: string}>>([]);

  $effect(() => {
    (async () => {
      try {
        const all = (await ListPlugins()) ?? [];
        const withSettings: Array<{name: string}> = [];
        for (const p of all) {
          try {
            // biome-ignore lint/performance/noAwaitInLoops: sequential schema fetch with per-plugin error isolation
            const schema = await GetPluginSettingsSchema(p.name);
            if (schema) {
              withSettings.push(p);
            }
          } catch {
            /* empty */
          }
        }
        plugins = withSettings;
      } catch {
        /* empty */
      }
    })();
  });

  function navTo(id: string) {
    push(id === "general" ? "/settings" : `/settings/${id}`);
  }

  function isActive(id: string): boolean {
    return active === id;
  }
</script>

<nav class="w-48 shrink-0 border-r border-border h-full overflow-y-auto py-2">
  <div class="px-3 pb-1"><span class="text-xs font-semibold uppercase tracking-wider text-muted">Settings</span></div>

  {#each mainSections as section}
    <button
      type="button"
      onclick={() => navTo(section.id)}
      class="w-full text-left px-3 py-1.5 text-sm transition-colors rounded-none
        {isActive(section.id)
          ? 'bg-accent text-accent-foreground'
          : 'text-fg hover:bg-surface-hover'}"
    >
      {section.label}
    </button>
  {/each}

  {#if plugins.length > 0}
    <div class="px-3 pt-3 pb-1"><span class="text-xs font-semibold uppercase tracking-wider text-muted">Plugins</span></div>
    <button
      type="button"
      onclick={() => push('/settings/plugins')}
      class="w-full text-left px-3 py-1.5 text-sm transition-colors rounded-none
        {isActive('plugins')
          ? 'bg-accent text-accent-foreground'
          : 'text-fg hover:bg-surface-hover'}"
    >
      All Plugins
    </button>
    {#each plugins as plugin}
      <button
        type="button"
        onclick={() => push(`/settings/plugins/${plugin.name}`)}
        class="w-full text-left px-3 py-1.5 text-sm transition-colors rounded-none pl-6
          {isActive(`plugins/${plugin.name}`)
            ? 'bg-accent text-accent-foreground'
            : 'text-muted hover:bg-surface-hover'}"
      >
        {plugin.name}
      </button>
    {/each}
  {/if}
</nav>
