<script lang="ts">
  import {onMount} from "svelte";
  import {
    SetAccentColor,
    SetCompactRows,
    SetContextualAutocomplete,
  } from "$api/github.com/Vilsol/klados/internal/services/configservice.js";
  import {preferencesStore} from "$lib/stores/preferences.svelte";

  const presetColors = ["#6366f1", "#8b5cf6", "#ec4899", "#ef4444", "#f97316", "#eab308", "#22c55e", "#06b6d4"];

  let accentColor = $state<string>("");
  let compactRows = $state<boolean>(false);
  let contextualAutocomplete = $state<boolean>(true);

  onMount(() => {
    accentColor = preferencesStore.prefs.accentColor || "";
    compactRows = preferencesStore.prefs.compactRows;
    contextualAutocomplete = preferencesStore.prefs.contextualAutocomplete;
  });

  function setAccent(color: string) {
    accentColor = color;
    SetAccentColor(color);
  }

  function setCompact(checked: boolean) {
    compactRows = checked;
    SetCompactRows(checked);
  }

  function setContextualAutocomplete(checked: boolean) {
    contextualAutocomplete = checked;
    SetContextualAutocomplete(checked);
  }

  let isPreset = $derived(presetColors.includes(accentColor));
</script>

<div class="max-w-2xl space-y-8">
  <div>
    <h2 class="text-base font-medium text-fg mb-4">Accent Color</h2>
    <div class="flex items-center gap-3 flex-wrap">
      {#each presetColors as color}
        <button
          type="button"
          class="w-8 h-8 rounded-full border-2 transition-all {accentColor === color ? 'border-fg scale-110' : 'border-transparent hover:border-muted-foreground'}"
          style="background-color: {color}"
          onclick={() => setAccent(color)}
          aria-label="Set accent color to {color}"
        ></button>
      {/each}
      <label
        class="relative w-8 h-8 rounded-full border-2 overflow-hidden cursor-pointer {accentColor && !isPreset ? 'border-fg' : 'border-border'}"
        style="background-color: {accentColor && !isPreset ? accentColor : 'transparent'}"
      >
        <input
          type="color"
          value={accentColor || '#6366f1'}
          oninput={(e) => setAccent((e.target as HTMLInputElement).value)}
          class="absolute inset-0 opacity-0 cursor-pointer"
        >
        {#if !accentColor || isPreset}
          <span class="absolute inset-0 flex items-center justify-center text-muted-foreground text-xs">+</span>
        {/if}
      </label>
    </div>
    {#if accentColor}
      <button type="button" class="mt-3 text-sm text-muted-foreground hover:text-fg underline" onclick={() => setAccent('')}>
        Reset to default
      </button>
    {/if}
  </div>

  <div>
    <h2 class="text-base font-medium text-fg mb-4">Compact Rows</h2>
    <label class="flex items-center gap-2 cursor-pointer">
      <input
        type="checkbox"
        checked={compactRows}
        onchange={(e) => setCompact((e.target as HTMLInputElement).checked)}
        class="accent-accent"
      >
      <span class="text-sm text-fg">Reduce row height in resource lists</span>
    </label>
  </div>

  <div>
    <h2 class="text-base font-medium text-fg mb-4">Contextual Autocomplete</h2>
    <label class="flex items-center gap-2 cursor-pointer">
      <input
        type="checkbox"
        checked={contextualAutocomplete}
        onchange={(e) => setContextualAutocomplete((e.target as HTMLInputElement).checked)}
        class="accent-accent"
      >
      <span class="text-sm text-fg">Autocomplete suggestions reflect active search filters</span>
    </label>
  </div>
</div>
