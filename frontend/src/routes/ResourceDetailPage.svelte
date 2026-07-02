<script lang="ts">
  import {push} from "svelte-spa-router";
  import {GetResource} from "$api/github.com/Vilsol/klados/internal/services/resourceservice.js";
  import {descriptorRegistry} from "$lib/registry/index";
  import {registryLoaded} from "$lib/registry/loaded.svelte";
  import ResourceDetail from "$lib/components/ResourceDetail.svelte";
  import {clusterStore} from "$lib/stores/cluster.svelte";

  let {params = {}}: {params?: Record<string, string>} = $props();

  const ctxName = $derived(params.ctx ?? "");

  $effect(() => {
    if (ctxName) {
      clusterStore.setActiveContext(ctxName);
    }
  });
  const gvr = $derived(params.gvr ?? "");
  const ns = $derived(params.ns ?? "");
  const name = $derived(params.name ?? "");

  const resourceLabel = $derived(gvr.split(".").at(-1) ?? gvr);

  let obj = $state<Record<string, unknown> | null>(null);
  let loading = $state(true);
  let error = $state<string | null>(null);

  async function load() {
    if (!(ctxName && gvr && name)) {
      return;
    }
    loading = true;
    error = null;
    try {
      obj = (await GetResource(ctxName, gvr, ns, name)) as Record<string, unknown> | null;
    } catch (e: unknown) {
      error = e instanceof Error ? e.message : String(e);
    } finally {
      loading = false;
    }
  }

  $effect(() => {
    void ctxName;
    void gvr;
    void ns;
    void name;
    load();
  });

  const descriptor = $derived(registryLoaded() ? descriptorRegistry.get(gvr) : null);
</script>

<div class="flex flex-col h-full overflow-hidden">
  <!-- Breadcrumb -->
  <div class="shrink-0 px-4 py-2 border-b border-border flex items-center gap-1.5 text-xs text-muted overflow-x-auto">
    <button type="button" onclick={() => push(`/c/${ctxName}`)} class="hover:text-fg transition-colors whitespace-nowrap">{ctxName}</button>
    <span>/</span>
    <button type="button" onclick={() => push(`/c/${ctxName}/${gvr}`)} class="hover:text-fg transition-colors whitespace-nowrap">
      {resourceLabel}
    </button>
    {#if ns}
      <span>/</span>
      <span class="whitespace-nowrap">{ns}</span>
    {/if}
    <span>/</span>
    <span class="text-fg font-medium whitespace-nowrap">{name}</span>
  </div>

  <!-- Content -->
  {#if loading}
    <div class="p-4 text-sm text-muted">Loading...</div>
  {:else if error}
    <div class="p-4 text-sm text-destructive">{error}</div>
  {:else if obj && descriptor}
    <div class="flex-1 overflow-hidden">
      <ResourceDetail
        bind:obj
        {descriptor}
        {ctxName}
        {gvr}
        namespace={ns}
        {name}
        onrefresh={load}
        onopenresource={(g, n2, nm) => push(`/c/${encodeURIComponent(ctxName)}/${g}/${encodeURIComponent(n2)}/${encodeURIComponent(nm)}`)}
      />
    </div>
  {:else if obj}
    <pre
      class="flex-1 overflow-auto text-xs font-mono bg-surface rounded border border-border m-4 p-3 whitespace-pre-wrap"
    >{JSON.stringify(obj, null, 2)}</pre>
  {/if}
</div>
