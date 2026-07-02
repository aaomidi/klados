<script lang="ts">
  import {onMount} from "svelte";
  import {Events} from "@wailsio/runtime";
  import {ArrowDownToLine} from "lucide-svelte";
  import LogsPanel from "./panels/LogsPanel.svelte";
  import TerminalPanel from "./panels/TerminalPanel.svelte";
  import AggregateLogsPanel from "./panels/AggregateLogsPanel.svelte";
  import {YAMLEditor} from "@klados/ui";
  import {UpdateResource, GetResource} from "$api/github.com/Vilsol/klados/internal/services/resourceservice.js";
  import {GetSchema} from "$api/github.com/Vilsol/klados/internal/services/schemaservice.js";
  import {notificationStore} from "$lib/stores/notification.svelte.js";
  import {shortcutStore} from "$lib/stores/shortcuts.svelte.js";
  import {unwrapError} from "$lib/utils/async.js";
  import type {PanelKind} from "$lib/stores/bottom-panel.svelte";

  let {panelId}: {panelId: string} = $props();

  interface PanelData {
    kind: PanelKind;
    resourceKind: string;
    resourceName: string;
    ctxName: string;
    gvr: string;
    namespace: string;
    name: string;
    obj: Record<string, unknown>;
  }

  let panelData = $state<PanelData | null>(null);

  onMount(() => {
    const unsub = Events.On(`panel:init:${panelId}`, (event: {data: PanelData}) => {
      panelData = event.data;
    });

    Events.Emit("panel:ready", panelId);

    return () => unsub();
  });

  function popIn() {
    Events.Emit("panel:pop-in", panelId);
  }
</script>

<div class="flex flex-col h-screen bg-bg text-fg">
  <!-- Mini toolbar -->
  <div class="flex items-center gap-2 px-2 py-1 border-b border-border bg-surface shrink-0">
    <button
      type="button"
      onclick={popIn}
      class="flex items-center gap-1 px-2 py-1 text-xs rounded hover:bg-surface-hover text-muted hover:text-fg transition-colors"
      title="Return to bottom panel"
    >
      <ArrowDownToLine size={12} />
      <span>Pop in</span>
    </button>
    {#if panelData}
      <span class="text-xs text-muted truncate">{panelData.resourceKind}: {panelData.resourceName}</span>
    {/if}
  </div>

  <!-- Panel content -->
  <div class="flex-1 overflow-hidden">
    {#if panelData}
      {#if panelData.kind === 'logs'}
        <LogsPanel obj={panelData.obj} ctxName={panelData.ctxName} namespace={panelData.namespace} name={panelData.name} />
      {:else if panelData.kind === 'terminal'}
        <TerminalPanel obj={panelData.obj} ctxName={panelData.ctxName} namespace={panelData.namespace} name={panelData.name} />
      {:else if panelData.kind === 'aggregate-logs'}
        <AggregateLogsPanel obj={panelData.obj} ctxName={panelData.ctxName} namespace={panelData.namespace} name={panelData.name} />
      {:else if panelData.kind === 'yaml'}
        <YAMLEditor
          obj={panelData.obj}
          ctxName={panelData.ctxName}
          gvr={panelData.gvr}
          namespace={panelData.namespace}
          name={panelData.name}
          kind={panelData.resourceKind}
          onSave={(ctx: string, g: string, ns: string, parsed: Record<string, unknown>) => UpdateResource(ctx, g, ns, parsed)}
          onGetResource={(ctx: string, g: string, ns: string, n: string) => GetResource(ctx, g, ns, n)}
          onGetSchema={(ctx: string, g: string, k: string) => GetSchema(ctx, g, k)}
          onSetEditorMode={(mode: string) => shortcutStore.setMode(mode as 'normal' | 'editor')}
          onNotify={(msg: string, type: 'info' | 'success' | 'error') => {
            if (type === 'success') { notificationStore.success(msg); }
            else if (type === 'error') { notificationStore.error(unwrapError(msg)); }
            else { notificationStore.push(msg, type); }
          }}
        />
      {/if}
    {:else}
      <div class="flex items-center justify-center h-full text-sm text-muted">Loading panel...</div>
    {/if}
  </div>
</div>
