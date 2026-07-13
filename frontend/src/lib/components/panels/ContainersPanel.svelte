<script lang="ts">
  import {onMount} from "svelte";
  import {ExternalLink, X} from "lucide-svelte";
  import PortForwardDialog from "$lib/components/PortForwardDialog.svelte";
  import ContainerSections from "./ContainerSections.svelte";
  import {SectionHeader, DataTable} from "@klados/ui";
  import {portForwardStore} from "$lib/stores/portforward.svelte";
  import {StopForward} from "$api/github.com/Vilsol/klados/internal/services/portforwardservice.js";
  import {Browser} from "@wailsio/runtime";
  import {notificationStore} from "$lib/stores/notification.svelte";
  import {unwrapError} from "$lib/utils/async.js";
  import type {KubernetesResource} from "$lib/types";

  let {
    obj,
    ctxName = "",
    onopenresource,
    onopencontainer,
  }: {
    obj: Record<string, KubernetesResource>;
    ctxName?: string;
    onopenresource?: (gvr: string, namespace: string, name: string) => void;
    onopencontainer?: (panel: "logs" | "terminal", container: string) => void;
  } = $props();

  let pfPort = $state<number | null>(null);

  const conditions = $derived<KubernetesResource[]>(obj.status?.conditions ?? []);

  const podNamespace = $derived<string>(obj.metadata?.namespace ?? "");
  const podName = $derived<string>(obj.metadata?.name ?? "");

  onMount(() => {
    let release: (() => void) | undefined;
    (async () => {
      if (ctxName) {
        release = await portForwardStore.subscribe(ctxName);
      }
    })();
    return () => release?.();
  });

  const activeForwards = $derived(
    ctxName ? portForwardStore.forPod(ctxName, podNamespace, podName) : [],
  );

  function forwardForPort(containerPort: number) {
    return activeForwards.find((f) => f.remotePort === containerPort);
  }

  async function stopForward(id: string) {
    try {
      await StopForward(id);
    } catch (e) {
      notificationStore.error("Failed to stop port forward", unwrapError(e));
    }
  }
</script>

<div class="flex flex-col gap-4 p-4 overflow-auto">
  {#if activeForwards.length > 0}
    <section>
      <SectionHeader>Active Port Forwards</SectionHeader>
      <div class="flex flex-col gap-1">
        {#each activeForwards as fwd (fwd.id)}
          <div class="flex items-center gap-2 bg-surface border border-border rounded px-2 py-1.5">
            <span
              class="inline-block w-1.5 h-1.5 rounded-full {fwd.status === 'active' ? 'bg-green-500' : fwd.status === 'reconnecting' ? 'bg-yellow-500' : 'bg-red-500'}"
              aria-label={fwd.status}
            ></span>
            <span class="text-xs font-mono">localhost:{fwd.localPort} → :{fwd.remotePort}</span>
            {#if fwd.error}
              <span class="text-xs text-red-500 truncate" title={fwd.error}>({fwd.error})</span>
            {/if}
            <div class="ml-auto flex items-center gap-1">
              {#if fwd.status === 'active' && fwd.localPort > 0}
                <button
                  type="button"
                  onclick={() => Browser.OpenURL(`http://localhost:${fwd.localPort}`)}
                  class="p-1 rounded text-muted hover:text-fg hover:bg-surface-hover transition-colors"
                  title="Open in browser"
                  aria-label="Open in browser"
                >
                  <ExternalLink size={12} />
                </button>
              {/if}
              <button
                type="button"
                onclick={() => stopForward(fwd.id)}
                class="p-1 rounded text-muted hover:text-fg hover:bg-surface-hover transition-colors"
                title="Stop port forward"
                aria-label="Stop port forward"
              >
                <X size={12} />
              </button>
            </div>
          </div>
        {/each}
      </div>
    </section>
  {/if}

  <ContainerSections {obj} {onopenresource} {onopencontainer} onforwardport={(p) => (pfPort = p)}>
    {#snippet portBadge(p)}
      {@const fwd = forwardForPort(p.containerPort)}
      {#if fwd}
        <button
          type="button"
          onclick={() => { if (fwd.status === 'active' && fwd.localPort > 0) Browser.OpenURL(`http://localhost:${fwd.localPort}`); }}
          class="inline-flex items-center gap-1 text-xs font-mono rounded px-1.5 py-0.5 border
            {fwd.status === 'active' ? 'bg-green-500/10 border-green-500/30 text-green-400 hover:bg-green-500/20' :
             fwd.status === 'reconnecting' ? 'bg-yellow-500/10 border-yellow-500/30 text-yellow-400' :
             'bg-red-500/10 border-red-500/30 text-red-400'}"
          title="Forwarded to localhost:{fwd.localPort} ({fwd.status})"
        >
          <span class="inline-block w-1.5 h-1.5 rounded-full
            {fwd.status === 'active' ? 'bg-green-500' :
             fwd.status === 'reconnecting' ? 'bg-yellow-500' :
             'bg-red-500'}"></span>
          :{fwd.localPort || '?'}
        </button>
      {/if}
    {/snippet}
  </ContainerSections>

  <!-- Pod conditions -->
  {#if conditions.length > 0}
    <section>
      <SectionHeader>Conditions</SectionHeader>
      <DataTable columns={[{ label: 'Type' }, { label: 'Status' }, { label: 'Reason' }]} items={conditions}>
        {#snippet row(cond: KubernetesResource)}
          <td class="px-2 py-1.5 font-mono">{cond.type}</td>
          <td class="px-2 py-1.5">
            <span class="{cond.status === 'True' ? 'text-green-600 dark:text-green-400' : 'text-muted'}">{cond.status}</span>
          </td>
          <td class="px-2 py-1.5 text-muted">{cond.reason ?? '—'}</td>
        {/snippet}
      </DataTable>
    </section>
  {/if}
</div>

{#if pfPort !== null}
  <PortForwardDialog
    prefillContext={ctxName}
    prefillNamespace={obj.metadata?.namespace ?? ''}
    prefillTargetKind="pod"
    prefillTarget={obj.metadata?.name ?? ''}
    prefillGVR=""
    prefillRemotePort={pfPort}
    onclose={() => pfPort = null}
  />
{/if}
