<script lang="ts">
  import type {Snippet} from "svelte";
  import {SectionHeader} from "@klados/ui";
  import ContainerCard from "./ContainerCard.svelte";
  import {groupContainers} from "$lib/kubernetes/containers";
  import type {KubernetesResource} from "$lib/types";

  let {
    obj,
    onopenresource,
    onopencontainer,
    onforwardport,
    portBadge,
  }: {
    obj: Record<string, KubernetesResource>;
    onopenresource?: (gvr: string, namespace: string, name: string) => void;
    onopencontainer?: (panel: "logs" | "terminal", container: string) => void;
    onforwardport?: (port: number) => void;
    portBadge?: Snippet<[KubernetesResource]>;
  } = $props();

  const groups = $derived(groupContainers(obj.spec));
  const volumes = $derived<KubernetesResource[]>(obj.spec?.volumes ?? []);
  const namespace = $derived<string>(obj.metadata?.namespace ?? "");

  function statusOf(name: string): KubernetesResource {
    return (obj.status?.containerStatuses ?? []).find((s: KubernetesResource) => s.name === name);
  }
  function initStatusOf(name: string): KubernetesResource {
    return (obj.status?.initContainerStatuses ?? []).find((s: KubernetesResource) => s.name === name);
  }
  function ephemeralStatusOf(name: string): KubernetesResource {
    return (obj.status?.ephemeralContainerStatuses ?? []).find((s: KubernetesResource) => s.name === name);
  }

  let showInit = $state(false);
</script>

{#if groups.containers.length > 0}
  <section class="bg-surface border border-border rounded-lg p-4">
    <SectionHeader class="mb-3">Containers</SectionHeader>
    <div class="flex flex-col gap-3">
      {#each groups.containers as c (c.name)}
        <ContainerCard container={c} status={statusOf(c.name)} {volumes} {namespace} {onopenresource} {onopencontainer} {onforwardport} {portBadge} />
      {/each}
    </div>
  </section>
{/if}

{#if groups.sidecars.length > 0}
  <section class="bg-surface border border-border rounded-lg p-4">
    <SectionHeader class="mb-3">Sidecars ({groups.sidecars.length})</SectionHeader>
    <div class="flex flex-col gap-3">
      {#each groups.sidecars as c (c.name)}
        <ContainerCard container={c} status={initStatusOf(c.name)} {volumes} {namespace} {onopenresource} {onopencontainer} {onforwardport} {portBadge} />
      {/each}
    </div>
  </section>
{/if}

{#if groups.ephemeral.length > 0}
  <section class="bg-surface border border-border rounded-lg p-4">
    <SectionHeader class="mb-3">Ephemeral Containers ({groups.ephemeral.length})</SectionHeader>
    <div class="flex flex-col gap-3">
      {#each groups.ephemeral as c (c.name)}
        <ContainerCard container={c} status={ephemeralStatusOf(c.name)} {volumes} {namespace} {onopenresource} {onopencontainer} />
      {/each}
    </div>
  </section>
{/if}

{#if groups.init.length > 0}
  <section class="bg-surface border border-border rounded-lg p-4">
    <button
      type="button"
      onclick={() => (showInit = !showInit)}
      class="text-xs font-semibold text-muted uppercase tracking-wide flex items-center gap-1"
    >
      {showInit ? '▾' : '▸'}
      Init Containers ({groups.init.length})
    </button>
    {#if showInit}
      <div class="flex flex-col gap-2 mt-3">
        {#each groups.init as c (c.name)}
          <ContainerCard container={c} status={initStatusOf(c.name)} {volumes} {namespace} {onopenresource} {onopencontainer} compact />
        {/each}
      </div>
    {/if}
  </section>
{/if}
