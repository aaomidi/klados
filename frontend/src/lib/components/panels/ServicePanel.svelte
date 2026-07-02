<script lang="ts">
  import {onMount} from "svelte";
  import {GetResource} from "$api/github.com/Vilsol/klados/internal/services/resourceservice.js";
  import PortForwardDialog from "$lib/components/PortForwardDialog.svelte";
  import {SectionHeader, KeyValueBadge, EmptyState} from "@klados/ui";
  import type {KubernetesResource} from "$lib/types";

  let {obj, ctxName}: {obj: Record<string, KubernetesResource>; ctxName: string} = $props();

  let pfPort = $state<number | null>(null);

  const selector = $derived<Record<string, string>>(obj.spec?.selector ?? {});
  const ports = $derived<KubernetesResource[]>(obj.spec?.ports ?? []);

  let endpoints = $state<KubernetesResource | null>(null);
  let endpointError = $state("");

  const ns = $derived<string>(obj.metadata?.namespace ?? "");
  const svcName = $derived<string>(obj.metadata?.name ?? "");

  $effect(() => {
    if (!(ctxName && ns && svcName)) {
      return;
    }
    endpoints = null;
    endpointError = "";
    GetResource(ctxName, "core.v1.endpoints", ns, svcName)
      .then((r: unknown) => {
        endpoints = r;
      })
      .catch((e: unknown) => {
        endpointError = String(e);
      });
  });

  const subsets = $derived<KubernetesResource[]>(endpoints?.subsets ?? []);

  interface BackingPod {
    name: string;
    ip: string;
  }

  const backingPods = $derived<BackingPod[]>(
    subsets.flatMap((s: KubernetesResource) =>
      (s.addresses ?? []).map((a: KubernetesResource) => ({
        name: a.targetRef?.name ?? a.ip,
        ip: a.ip,
      })),
    ),
  );
</script>

<div class="flex flex-col gap-4 p-4 overflow-auto h-full">
  <!-- Selector -->
  <section>
    <SectionHeader>Selector</SectionHeader>
    {#if Object.keys(selector).length > 0}
      <KeyValueBadge entries={Object.entries(selector)} />
    {:else}
      <EmptyState message="No selector" size="sm" />
    {/if}
  </section>

  <!-- Ports -->
  {#if ports.length > 0}
    <section>
      <SectionHeader>Ports</SectionHeader>
      <table class="w-full text-sm">
        <thead>
          <tr class="text-left text-muted text-xs border-b border-border">
            <th class="pb-1 pr-4 font-medium">Name</th>
            <th class="pb-1 pr-4 font-medium">Port</th>
            <th class="pb-1 pr-4 font-medium">Protocol</th>
            <th class="pb-1 pr-4 font-medium">Target</th>
            <th class="pb-1 font-medium"></th>
          </tr>
        </thead>
        <tbody>
          {#each ports as p}
            <tr class="border-b border-border/50">
              <td class="py-1.5 pr-4 text-muted">{p.name ?? '—'}</td>
              <td class="py-1.5 pr-4 font-mono">{p.port}</td>
              <td class="py-1.5 pr-4">{p.protocol ?? 'TCP'}</td>
              <td class="py-1.5 pr-4 font-mono">{p.targetPort ?? '—'}</td>
              <td class="py-1.5">
                <button
                  type="button"
                  onclick={() => pfPort = p.port}
                  class="text-xs font-mono text-accent hover:underline"
                  title="Forward port {p.port}"
                  aria-label="Forward port {p.port}"
                >
                  ↗
                </button>
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </section>
  {/if}

  <!-- Endpoints / backing pods -->
  <section>
    <SectionHeader>Endpoints</SectionHeader>
    {#if endpointError}
      <EmptyState message={endpointError} size="sm" />
    {:else if endpoints === null}
      <EmptyState message="Loading…" size="sm" />
    {:else if backingPods.length === 0}
      <EmptyState message="No endpoints" size="sm" />
    {:else}
      <div class="flex flex-col gap-1">
        {#each backingPods as pod}
          <div class="flex items-center gap-2 text-sm">
            <span class="font-medium">{pod.name}</span>
            <span class="text-muted font-mono text-xs">{pod.ip}</span>
          </div>
        {/each}
      </div>
    {/if}
  </section>
</div>

{#if pfPort !== null}
  <PortForwardDialog
    prefillContext={ctxName}
    prefillNamespace={ns}
    prefillTargetKind="selector"
    prefillTarget={svcName}
    prefillGVR="core.v1.services"
    prefillRemotePort={pfPort}
    onclose={() => pfPort = null}
  />
{/if}
