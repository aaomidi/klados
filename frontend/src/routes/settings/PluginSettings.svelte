<script lang="ts">
  import {onMount} from "svelte";
  import {
    GetPluginSettingsSchema,
    GetPluginSettings,
    SetPluginSettings,
  } from "$api/github.com/Vilsol/klados/internal/services/pluginservice.js";
  import SchemaForm from "$lib/components/SchemaForm.svelte";
  import {getLogger} from "$lib/logger";

  const log = getLogger("settings");

  interface Props {
    pluginName: string;
  }

  let {pluginName}: Props = $props();

  let schema = $state<Record<string, unknown> | null>(null);
  let values = $state<Record<string, unknown>>({});
  let loading = $state<boolean>(true);

  onMount(() => {
    (async () => {
      try {
        const schemaStr = await GetPluginSettingsSchema(pluginName);
        if (schemaStr) {
          schema = JSON.parse(schemaStr) as Record<string, unknown>;
        }
        const settingsStr = await GetPluginSettings(pluginName);
        if (settingsStr) {
          values = JSON.parse(settingsStr) as Record<string, unknown>;
        }
      } catch (e) {
        log.error("Failed to load plugin settings", {error: String(e)});
      } finally {
        loading = false;
      }
    })();
  });

  function handleChange(key: string, value: unknown) {
    values = {...values, [key]: value};
    SetPluginSettings(pluginName, JSON.stringify(values));
  }
</script>

<div class="max-w-2xl space-y-6">
  <h2 class="text-base font-medium text-fg mb-4">{pluginName}</h2>

  {#if loading}
    <p class="text-sm text-muted-foreground">Loading settings...</p>
  {:else if !schema}
    <p class="text-sm text-muted-foreground">This plugin has no configurable settings.</p>
  {:else}
    <SchemaForm {schema} {values} onchange={handleChange} />
  {/if}
</div>
