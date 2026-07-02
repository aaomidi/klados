<script lang="ts">
  import {ListResources} from "$api/github.com/Vilsol/klados/internal/services/resourceservice.js";
  import type {KubernetesResource} from "$lib/types";

  let {obj, ctxName}: {obj: Record<string, KubernetesResource>; ctxName: string} = $props();

  const spec = $derived(obj.spec ?? {});
  const attachRequired = $derived<boolean>(spec.attachRequired ?? true);
  const podInfoOnMount = $derived<boolean>(spec.podInfoOnMount ?? false);
  const storageCapacity = $derived<boolean>(spec.storageCapacity ?? false);
  const fsGroupPolicy = $derived<string>(spec.fsGroupPolicy ?? "ReadWriteOnceWithFSType");
  const volumeLifecycleModes = $derived<string[]>(spec.volumeLifecycleModes ?? ["Persistent"]);

  type SnapshotStatus = "loading" | "unknown" | "none" | {classes: string[]};
  let snapshotStatus = $state<SnapshotStatus>("loading");

  $effect(() => {
    const driverName = obj.metadata?.name;
    (async () => {
      try {
        const items: KubernetesResource[] = (await ListResources(ctxName, "snapshot.storage.k8s.io.v1.volumesnapshotclasses", "")) ?? [];
        const matching = items.filter((c) => c.driver === driverName).map((c) => c.metadata?.name ?? "");
        snapshotStatus = matching.length > 0 ? {classes: matching} : "none";
      } catch {
        snapshotStatus = "unknown";
      }
    })();
  });

  function boolStr(v: boolean): string {
    return v ? "true" : "false";
  }
</script>

<div class="divide-y divide-border text-sm overflow-auto h-full">
  <div class="flex px-4 py-2">
    <span class="w-1/2 text-muted">Attach Required</span>
    <span class="w-1/2 font-mono">{boolStr(attachRequired)}</span>
  </div>
  <div class="flex px-4 py-2">
    <span class="w-1/2 text-muted">Pod Info On Mount</span>
    <span class="w-1/2 font-mono">{boolStr(podInfoOnMount)}</span>
  </div>
  <div class="flex px-4 py-2">
    <span class="w-1/2 text-muted">Storage Capacity</span>
    <span class="w-1/2 font-mono">{boolStr(storageCapacity)}</span>
  </div>
  <div class="flex px-4 py-2">
    <span class="w-1/2 text-muted">FS Group Policy</span>
    <span class="w-1/2 font-mono">{fsGroupPolicy}</span>
  </div>
  <div class="flex px-4 py-2">
    <span class="w-1/2 text-muted">Volume Lifecycle Modes</span>
    <span class="w-1/2 font-mono">{volumeLifecycleModes.join(', ')}</span>
  </div>
  <div class="flex px-4 py-2">
    <span class="w-1/2 text-muted">Snapshot Support</span>
    <span class="w-1/2">
      {#if snapshotStatus === 'loading'}
        <span class="text-muted">Checking…</span>
      {:else if snapshotStatus === 'unknown'}
        <span class="text-muted">Unknown — snapshot controller not installed</span>
      {:else if snapshotStatus === 'none'}
        <span class="text-muted">Not configured</span>
      {:else}
        <span
          >Supported ({snapshotStatus.classes.length} {snapshotStatus.classes.length === 1 ? 'class' : 'classes'}:
          {snapshotStatus.classes.join(', ')})</span
        >
      {/if}
    </span>
  </div>
</div>
