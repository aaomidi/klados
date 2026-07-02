<script lang="ts">
  import {SetKeybinding, ResetKeybindings} from "$api/github.com/Vilsol/klados/internal/services/configservice.js";
  import {shortcutStore, type ShortcutDef} from "$lib/stores/shortcuts.svelte";

  let listeningId = $state<string | null>(null);

  let shortcuts = $derived(shortcutStore.getAll());

  let effectiveMap = $derived.by(() => {
    const map = new Map<string, string>();
    for (const def of shortcuts) {
      map.set(def.id, shortcutStore.getEffectiveKeys(def));
    }
    return map;
  });

  let conflicts = $derived.by(() => {
    const comboCounts = new Map<string, string[]>();
    for (const [id, combo] of effectiveMap) {
      if (!combo) {
        continue;
      }
      const list = comboCounts.get(combo) ?? [];
      list.push(id);
      comboCounts.set(combo, list);
    }
    const conflictIds = new Set<string>();
    for (const ids of comboCounts.values()) {
      if (ids.length > 1) {
        for (const id of ids) {
          conflictIds.add(id);
        }
      }
    }
    return conflictIds;
  });

  function startListening(id: string) {
    listeningId = id;
  }

  function handleKeydown(e: KeyboardEvent) {
    if (!listeningId) {
      return;
    }
    e.preventDefault();
    e.stopPropagation();

    if (e.key === "Escape") {
      listeningId = null;
      return;
    }

    if (["Control", "Alt", "Shift", "Meta"].includes(e.key)) {
      return;
    }

    const hasModifier = e.ctrlKey || e.altKey || e.shiftKey || e.metaKey;
    if (!hasModifier) {
      return;
    }

    const parts: string[] = [];
    if (e.ctrlKey) {
      parts.push("Control");
    }
    if (e.altKey) {
      parts.push("Alt");
    }
    if (e.shiftKey) {
      parts.push("Shift");
    }
    if (e.metaKey) {
      parts.push("Meta");
    }
    parts.push(e.key);
    const combo = parts.join("+");

    SetKeybinding(listeningId, combo);
    listeningId = null;
  }

  function resetBinding(id: string) {
    SetKeybinding(id, "");
  }

  function resetAll() {
    ResetKeybindings();
  }

  function isOverridden(def: ShortcutDef): boolean {
    const effective = effectiveMap.get(def.id) ?? def.keys;
    return effective !== def.keys;
  }
</script>

<svelte:window onkeydown={handleKeydown} />

<div class="max-w-3xl space-y-6">
  <div class="flex items-center justify-between">
    <h2 class="text-base font-medium text-fg">Keyboard Shortcuts</h2>
    <button type="button" class="px-3 py-1.5 rounded border border-border text-fg text-sm hover:bg-surface-hover" onclick={resetAll}>
      Reset all to defaults
    </button>
  </div>

  <div class="border border-border rounded overflow-hidden">
    <table class="w-full text-sm">
      <thead>
        <tr class="bg-surface text-muted-foreground border-b border-border">
          <th class="text-left px-3 py-2 font-medium">Action</th>
          <th class="text-left px-3 py-2 font-medium">Current</th>
          <th class="text-left px-3 py-2 font-medium">Default</th>
          <th class="px-3 py-2 w-16"></th>
        </tr>
      </thead>
      <tbody>
        {#each shortcuts as def}
          {@const effective = effectiveMap.get(def.id) ?? def.keys}
          {@const hasConflict = conflicts.has(def.id)}
          <tr class="border-b border-border last:border-0">
            <td class="px-3 py-2 text-fg">{def.description}</td>
            <td class="px-3 py-2">
              <button
                type="button"
                class="px-2 py-0.5 rounded text-xs font-mono {listeningId === def.id ? 'bg-accent text-accent-foreground animate-pulse' : hasConflict ? 'bg-destructive/20 text-destructive border border-destructive/50' : 'bg-surface border border-border text-fg hover:bg-surface-hover'}"
                onclick={() => startListening(def.id)}
              >
                {listeningId === def.id ? 'Press keys...' : effective}
              </button>
            </td>
            <td class="px-3 py-2 text-muted-foreground font-mono text-xs">{def.keys}</td>
            <td class="px-3 py-2">
              {#if isOverridden(def)}
                <button type="button" class="text-xs text-muted-foreground hover:text-fg underline" onclick={() => resetBinding(def.id)}>
                  Reset
                </button>
              {/if}
            </td>
          </tr>
        {/each}
      </tbody>
    </table>
  </div>

  {#if shortcuts.length === 0}
    <p class="text-sm text-muted-foreground">No keyboard shortcuts registered.</p>
  {/if}
</div>
