<script lang="ts">
  import {evalExpr} from "$lib/registry/index";
  import {getFinalizers, LAST_APPLIED_ANNOTATION} from "$lib/kubernetes/metadata";
  import type {DescriptorDef} from "$lib/registry/index";
  import {formatAge} from "$lib/utils/age";
  import {getControllerRef, type ControllerRef} from "$lib/utils/relationships";
  import {clusterStore} from "$lib/stores/cluster.svelte";

  import {SectionHeader, KeyValueBadge, EmptyState, KeyValuePairEditor, CopyableValue} from "@klados/ui";
  import {slotRegistry} from "$lib/plugins/slots.svelte.js";
  import OwnerChain from "./OwnerChain.svelte";
  import {loadPluginComponent} from "$lib/plugins/loader.js";
  import {streamingStore} from "$lib/stores/streaming.svelte.js";
  import {UpdateResource} from "$api/github.com/Vilsol/klados/internal/services/resourceservice.js";
  import {notificationStore} from "$lib/stores/notification.svelte";
  import PortForwardDialog from "$lib/components/PortForwardDialog.svelte";
  import ContainerSections from "./ContainerSections.svelte";
  import type {KubernetesResource} from "$lib/types";

  let {
    obj,
    onupdate,
    descriptor,
    gvr = "",
    ctxName = "",
    namespace = "",
    name = "",
    onopenowner,
    onopenresource,
    onopencontainer,
  }: {
    obj: Record<string, KubernetesResource>;
    onupdate?: (updated: Record<string, KubernetesResource>) => void;
    descriptor: DescriptorDef;
    gvr?: string;
    ctxName?: string;
    namespace?: string;
    name?: string;
    onopenowner?: (ref: ControllerRef, namespace: string) => void;
    onopenresource?: (gvr: string, namespace: string, name: string) => void;
    onopencontainer?: (panel: "logs" | "terminal", container: string) => void;
  } = $props();

  const basePluginURL = $derived(streamingStore.config ? streamingStore.pluginsBaseUrl() : null);

  function getRawValue(expr: string): string {
    const raw = evalExpr(expr, obj);
    if (raw === null || raw === undefined) {
      return "";
    }
    return String(raw);
  }

  function renderValue(expr: string, renderType: string): string {
    const raw = evalExpr(expr, obj);
    if (renderType === "age" && raw) {
      return formatAge(String(raw));
    }
    if (raw === null || raw === undefined) {
      return "—";
    }
    return String(raw);
  }

  // Labels/annotations state
  const hasLabelsPanel = $derived(
    descriptor.detailPanels.includes("labels") || descriptor.detailPanels.includes("metadata"),
  );
  let editingLabels = $state(false);
  let saving = $state(false);
  let editLabels = $state<[string, string][]>([]);
  let editAnnotations = $state<[string, string][]>([]);

  function startEdit() {
    editLabels = Object.entries(obj.metadata?.labels ?? {}).map(([k, v]) => [k, String(v)]);
    editAnnotations = Object.entries(obj.metadata?.annotations ?? {}).map(([k, v]) => [k, String(v)]);
    editingLabels = true;
  }

  function cancelEdit() {
    editingLabels = false;
  }

  async function saveLabels() {
    saving = true;
    try {
      const updated = JSON.parse(JSON.stringify(obj));
      updated.metadata.labels = Object.fromEntries(editLabels.filter(([k]) => k.trim()));
      updated.metadata.annotations = Object.fromEntries(editAnnotations.filter(([k]) => k.trim()));
      const result = await UpdateResource(ctxName, gvr, namespace, updated);
      if (result) {
        onupdate?.(result);
      }
      editingLabels = false;
      notificationStore.push("Labels and annotations saved.", "success");
    } catch (e: unknown) {
      notificationStore.push(e instanceof Error ? e.message : "Save failed", "error");
    } finally {
      saving = false;
    }
  }

  const controllerRef = $derived(getControllerRef(obj));

  const tolerations = $derived<KubernetesResource[]>(obj.spec?.tolerations ?? obj.spec?.template?.spec?.tolerations ?? []);
  let tolerationsExpanded = $state(true);
  let tolerationsEl: HTMLElement | undefined = $state();

  function formatToleration(t: KubernetesResource): string {
    const key = t.key || "*";
    const op = t.operator === "Exists" ? "Exists" : `=${t.value ?? ""}`;
    const effect = t.effect || "All";
    const seconds = t.tolerationSeconds == null ? "" : ` (${t.tolerationSeconds}s)`;
    return `${key} ${op} — ${effect}${seconds}`;
  }

  function scrollToTolerations() {
    tolerationsExpanded = true;
    tolerationsEl?.scrollIntoView({behavior: "smooth", block: "nearest"});
  }

  const labels = $derived(Object.entries(obj.metadata?.labels ?? {}));
  const annotations = $derived(
    Object.entries(obj.metadata?.annotations ?? {}).filter(([k]) => k !== LAST_APPLIED_ANNOTATION),
  );

  // Containers state
  const hasContainersPanel = $derived(descriptor.detailPanels.includes("containers"));
  const conditions = $derived<KubernetesResource[]>(obj.status?.conditions ?? []);

  let pfPort = $state<number | null>(null);

  const finalizers = $derived(getFinalizers(obj));
</script>

<div class="overflow-auto h-full p-4 flex flex-col gap-4">
  <!-- Overview fields card -->
  <section class="bg-surface border border-border rounded-lg p-4">
    <SectionHeader class="mb-3">Details</SectionHeader>
    <OwnerChain contextName={ctxName} {obj} {onopenresource} />
    <div class="grid grid-cols-3 gap-x-6 gap-y-3">
      {#each descriptor.overviewFields as field}
        <div class="min-w-0">
          <div class="text-xs text-muted mb-0.5">{field.label}</div>
          {#if field.renderType === 'badge'}
            <CopyableValue value={renderValue(field.expr, field.renderType)} rawValue={getRawValue(field.expr)} class="text-xs font-mono">
              <span class="bg-bg border border-border rounded px-2 py-0.5 inline-block"> {renderValue(field.expr, field.renderType)} </span>
            </CopyableValue>
          {:else}
            <CopyableValue
              value={renderValue(field.expr, field.renderType)}
              rawValue={field.renderType === 'age' ? getRawValue(field.expr) : undefined}
              class="text-xs font-mono"
            />
          {/if}
        </div>
      {/each}
      {#if controllerRef}
        <div class="min-w-0">
          <div class="text-xs text-muted mb-0.5">Controlled By</div>
          {#if onopenowner && clusterStore.resolveOwnerGVR(controllerRef.apiVersion, controllerRef.kind)}
            <button
              type="button"
              class="text-xs font-mono text-accent hover:underline text-left"
              onclick={() => onopenowner?.(controllerRef, obj.metadata?.namespace ?? '')}
            >
              {controllerRef.kind}/{controllerRef.name}
            </button>
          {:else}
            <div class="text-xs font-mono truncate">{controllerRef.kind}/{controllerRef.name}</div>
          {/if}
        </div>
      {/if}
      {#if tolerations.length > 0}
        <div class="min-w-0">
          <div class="text-xs text-muted mb-0.5">Tolerations</div>
          <button type="button" onclick={scrollToTolerations} class="text-xs font-mono text-accent hover:underline">
            {tolerations.length}
          </button>
        </div>
      {/if}
    </div>
    {#if basePluginURL}
      {#each slotRegistry.getOverviewFields(gvr) as field (field.id)}
        {#await loadPluginComponent(field.pluginName, field.component, basePluginURL) then Cmp}
          {#if Cmp}
            <Cmp resource={obj} />
          {/if}
        {/await}
      {/each}
    {/if}
  </section>

  {#if tolerations.length > 0}
    <section bind:this={tolerationsEl} class="bg-surface border border-border rounded-lg p-4">
      <button type="button" onclick={() => tolerationsExpanded = !tolerationsExpanded} class="flex items-center gap-1 w-full text-left">
        <SectionHeader class="">{tolerationsExpanded ? '▾' : '▸'} Tolerations ({tolerations.length})</SectionHeader>
      </button>
      {#if tolerationsExpanded}
        <div class="flex flex-col gap-1 mt-3">
          {#each tolerations as t}
            <CopyableValue value={formatToleration(t)} class="text-xs font-mono" />
          {/each}
        </div>
      {/if}
    </section>
  {/if}

  <!-- Labels & Annotations card -->
  {#if hasLabelsPanel}
    <section class="bg-surface border border-border rounded-lg p-4">
      <div class="flex items-center justify-between mb-3">
        <SectionHeader class="">Labels & Annotations</SectionHeader>
        {#if !editingLabels}
          <button
            type="button"
            onclick={startEdit}
            class="text-xs px-2.5 py-1 rounded border border-border hover:bg-surface-hover transition-colors"
          >
            Edit
          </button>
        {:else}
          <div class="flex gap-2">
            <button
              type="button"
              onclick={cancelEdit}
              class="text-xs px-2.5 py-1 rounded border border-border hover:bg-surface-hover transition-colors"
            >
              Cancel
            </button>
            <button
              type="button"
              onclick={saveLabels}
              disabled={saving}
              class="text-xs px-2.5 py-1 rounded bg-accent text-accent-fg hover:opacity-90 transition-opacity disabled:opacity-50"
            >
              {saving ? 'Saving…' : 'Save'}
            </button>
          </div>
        {/if}
      </div>

      {#if !editingLabels}
        <div class="mb-3">
          <h4 class="text-xs font-medium mb-1.5">Labels</h4>
          <KeyValueBadge entries={labels} />
        </div>

        <div>
          <h4 class="text-xs font-medium mb-1.5">Annotations</h4>
          {#if annotations.length === 0}
            <EmptyState />
          {:else}
            <div class="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1.5">
              {#each annotations.sort(([a], [b]) => a.localeCompare(b)) as [ k, v ]}
                <span class="text-xs font-mono text-muted">{k}</span>
                <span class="text-xs font-mono break-all">{v}</span>
              {/each}
            </div>
          {/if}
        </div>
      {:else}
        <div class="mb-3">
          <h4 class="text-xs font-medium mb-1.5">Labels</h4>
          <KeyValuePairEditor bind:pairs={editLabels} addLabel="+ Add label" />
        </div>

        <div>
          <h4 class="text-xs font-medium mb-1.5">Annotations</h4>
          <KeyValuePairEditor bind:pairs={editAnnotations} addLabel="+ Add annotation" />
        </div>
      {/if}
    </section>
  {/if}

  <!-- Conditions card -->
  {#if hasContainersPanel && conditions.length > 0}
    <section class="bg-surface border border-border rounded-lg p-4">
      <SectionHeader class="mb-3">Conditions</SectionHeader>
      <div class="grid grid-cols-1 sm:grid-cols-3 gap-2">
        {#each conditions as cond}
          <div class="flex items-center gap-2 px-3 py-2 rounded-md bg-bg border border-border" title={cond.message ?? ''}>
            <span
              class="w-2 h-2 rounded-full shrink-0
              {cond.status === 'True' ? 'bg-green-500' : 'bg-muted'}"
            ></span>
            <span class="text-xs font-mono flex-1 truncate">{cond.type}</span>
            {#if cond.reason}
              <span class="text-xs text-muted truncate">{cond.reason}</span>
            {/if}
            {#if cond.lastTransitionTime}
              <span class="text-xs text-muted shrink-0 tabular-nums" title={new Date(cond.lastTransitionTime).toLocaleString()}
                >{formatAge(cond.lastTransitionTime)}</span
              >
            {/if}
          </div>
        {/each}
      </div>
    </section>
  {/if}

  <!-- Containers -->
  {#if hasContainersPanel}
    <ContainerSections {obj} {onopenresource} {onopencontainer} onforwardport={(p) => (pfPort = p)} />
  {/if}

  <!-- Finalizers -->
  {#if finalizers.length > 0}
    <section class="bg-surface border border-border rounded-lg p-4">
      <SectionHeader class="mb-3">Finalizers</SectionHeader>
      <div class="flex flex-wrap gap-2">
        {#each finalizers as f}
          <span class="rounded bg-bg border border-border px-2 py-0.5 font-mono text-xs">{f}</span>
        {/each}
      </div>
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
