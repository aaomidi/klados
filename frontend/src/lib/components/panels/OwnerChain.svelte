<script lang="ts">
  import {ChevronRight, Link2} from "lucide-svelte";
  import {getOwnerReferences, gvrFromAPIVersion} from "$lib/kubernetes/owners";
  import {GetResource} from "$api/github.com/Vilsol/klados/internal/services/resourceservice.js";
  import {push} from "svelte-spa-router";

  interface Props {
    contextName: string;
    obj: Record<string, unknown>;
    onopenresource?: (gvr: string, namespace: string, name: string) => void;
  }
  let {contextName, obj, onopenresource}: Props = $props();

  interface ChainNode {
    gvr: string;
    namespace: string;
    name: string;
    kind: string;
  }

  let chain = $state<ChainNode[]>([]);

  $effect(() => {
    const currentObj = obj;
    const ctx = contextName;
    (async () => {
      const result: ChainNode[] = [];
      let current: Record<string, unknown> | null = currentObj;
      for (let depth = 0; depth < 5; depth++) {
        const refs = getOwnerReferences(current);
        if (refs.length === 0) break;
        const controller = refs.find((r) => r.controller) ?? refs[0];
        const parentGvr = gvrFromAPIVersion(controller.apiVersion, controller.kind);
        const parentNs = (current as any)?.metadata?.namespace ?? "";
        result.push({gvr: parentGvr, namespace: parentNs, name: controller.name, kind: controller.kind});
        try {
          current = await GetResource(ctx, parentGvr, parentNs, controller.name) as Record<string, unknown>;
        } catch {
          break;
        }
      }
      chain = result;
    })();
  });

  function navigate(n: ChainNode) {
    if (onopenresource) {
      onopenresource(n.gvr, n.namespace, n.name);
    } else {
      push(`/c/${encodeURIComponent(contextName)}/${n.gvr}/${encodeURIComponent(n.namespace)}/${encodeURIComponent(n.name)}`);
    }
  }
</script>

{#if chain.length > 0}
  <div class="flex items-center gap-1 mb-3 text-xs min-w-0 overflow-hidden">
    <Link2 size={11} class="text-muted shrink-0" />
    <span class="text-muted shrink-0 mr-1">Owners</span>
    <div class="flex items-center gap-1 min-w-0 overflow-hidden">
      {#each chain as n, i}
        {#if i > 0}
          <ChevronRight size={11} class="text-muted shrink-0" />
        {/if}
        <button
          type="button"
          class="group flex items-center gap-1.5 px-1.5 py-0.5 rounded border border-border bg-bg hover:border-accent hover:bg-surface-hover transition-colors min-w-0 max-w-[180px]"
          onclick={() => navigate(n)}
          title="{n.kind}/{n.namespace ? `${n.namespace}/` : ''}{n.name}"
        >
          <span class="text-[10px] uppercase tracking-wide text-muted group-hover:text-accent shrink-0">{n.kind}</span>
          <span class="font-mono truncate">{n.name}</span>
        </button>
      {/each}
    </div>
  </div>
{/if}
