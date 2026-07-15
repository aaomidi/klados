<script lang="ts">
  import type {Component} from "svelte";
  import type {DescriptorDef} from "$lib/registry/index";
  import {slotRegistry} from "$lib/plugins/slots.svelte.js";
  import {loadPluginComponent} from "$lib/plugins/loader.js";
  import {createPluginContext} from "$lib/plugins/context.js";
  import type {PluginManifest} from "$lib/plugins/types/manifest.js";
  import {clusterStore} from "$lib/stores/cluster.svelte.js";
  import {streamingStore} from "$lib/stores/streaming.svelte.js";
  import {
    ListResources,
    GetResource,
    UpdateResource,
  } from "$api/github.com/Vilsol/klados/internal/services/resourceservice.js";
  import {GetSchema} from "$api/github.com/Vilsol/klados/internal/services/schemaservice.js";
  import {notificationStore} from "$lib/stores/notification.svelte.js";
  import {shortcutStore} from "$lib/stores/shortcuts.svelte.js";
  import {unwrapError} from "$lib/utils/async.js";
  import type {ControllerRef} from "$lib/utils/relationships";
  import {ExternalLink} from "lucide-svelte";
  import {bottomPanelStore, type PanelKind} from "$lib/stores/bottom-panel.svelte";

  import OverviewPanel from "./panels/OverviewPanel.svelte";
  import EventsPanel from "./panels/EventsPanel.svelte";
  import LabelsAnnotationsPanel from "./panels/LabelsAnnotationsPanel.svelte";
  import ContainersPanel from "./panels/ContainersPanel.svelte";
  import PodDiagnosisBanner from "./panels/PodDiagnosisBanner.svelte";
  import DeploymentPanel from "./panels/DeploymentPanel.svelte";
  import LogsPanel from "./panels/LogsPanel.svelte";
  import TerminalPanel from "./panels/TerminalPanel.svelte";
  import AggregateLogsPanel from "./panels/AggregateLogsPanel.svelte";
  import ServicePanel from "./panels/ServicePanel.svelte";
  import IngressPanel from "./panels/IngressPanel.svelte";
  import ConfigMapPanel from "./panels/ConfigMapPanel.svelte";
  import SecretPanel from "./panels/SecretPanel.svelte";
  import NodePanel from "./panels/NodePanel.svelte";
  import NodeDrainTab from "./panels/NodeDrainTab.svelte";
  import RulesPanel from "./panels/RulesPanel.svelte";
  import BindingPanel from "./panels/BindingPanel.svelte";
  import ServiceAccountPanel from "./panels/ServiceAccountPanel.svelte";
  import StorageClassParametersPanel from "./panels/StorageClassParametersPanel.svelte";
  import CSICapabilitiesPanel from "./panels/CSICapabilitiesPanel.svelte";
  import CRDPanel from "./panels/CRDPanel.svelte";
  import CRDSchemaPanel from "./panels/CRDSchemaPanel.svelte";
  import ResourceQuotaPanel from "./panels/ResourceQuotaPanel.svelte";
  import LimitRangePanel from "./panels/LimitRangePanel.svelte";
  import PDBPanel from "./panels/PDBPanel.svelte";
  import EndpointSlicePanel from "./panels/EndpointSlicePanel.svelte";
  import NetworkPolicyPanel from "./panels/NetworkPolicyPanel.svelte";
  import HPAPanel from "./panels/HPAPanel.svelte";
  import WebhookConfigPanel from "./panels/WebhookConfigPanel.svelte";
  import ConditionsPanel from "./panels/ConditionsPanel.svelte";
  import MetadataPanel from "./panels/MetadataPanel.svelte";
  import DriftPanel from "./panels/DriftPanel.svelte";
  import RelatedResourcesPanel from "./panels/RelatedResourcesPanel.svelte";
  import ActionsToolbar from "./panels/ActionsToolbar.svelte";
  import { getLastAppliedConfig } from "../kubernetes/metadata";
  import ValidationWarningBanner from "./ValidationWarningBanner.svelte";
  import MetricsTab from "./charts/MetricsTab.svelte";
  import {YAMLEditor} from "@klados/ui";
  import type {KubernetesResource} from "$lib/types";

  type PanelComponent = Component<KubernetesResource>;

  const panelComponents: Map<string, PanelComponent> = new Map([
    ["overview", OverviewPanel as PanelComponent],
    ["yaml", YAMLEditor as PanelComponent],
    ["events", EventsPanel as PanelComponent],
    ["labels", LabelsAnnotationsPanel as PanelComponent],
    ["containers", ContainersPanel as PanelComponent],
    ["deployment-detail", DeploymentPanel as PanelComponent],
    ["logs", LogsPanel as PanelComponent],
    ["terminal", TerminalPanel as PanelComponent],
    ["aggregate-logs", AggregateLogsPanel as PanelComponent],
    ["service", ServicePanel as PanelComponent],
    ["ingress", IngressPanel as PanelComponent],
    ["configmap", ConfigMapPanel as PanelComponent],
    ["secret", SecretPanel as PanelComponent],
    ["node", NodePanel as PanelComponent],
    ["drain", NodeDrainTab as PanelComponent],
    ["rules", RulesPanel as PanelComponent],
    ["binding", BindingPanel as PanelComponent],
    ["serviceaccount", ServiceAccountPanel as PanelComponent],
    ["sc-parameters", StorageClassParametersPanel as PanelComponent],
    ["csi-capabilities", CSICapabilitiesPanel as PanelComponent],
    ["crd", CRDPanel as PanelComponent],
    ["crd-schema", CRDSchemaPanel as PanelComponent],
    ["metrics", MetricsTab as PanelComponent],
    ["netpol", NetworkPolicyPanel as PanelComponent],
    ["endpointslice", EndpointSlicePanel as PanelComponent],
    ["resourcequota", ResourceQuotaPanel as PanelComponent],
    ["limitrange", LimitRangePanel as PanelComponent],
    ["hpa", HPAPanel as PanelComponent],
    ["pdb", PDBPanel as PanelComponent],
    ["webhooks", WebhookConfigPanel as PanelComponent],
    ["conditions", ConditionsPanel as PanelComponent],
    ["metadata", MetadataPanel as PanelComponent],
    ["drift", DriftPanel as PanelComponent],
    ["related", RelatedResourcesPanel as PanelComponent],
  ]);

  const panelLabels: Record<string, string> = {
    overview: "Overview",
    yaml: "YAML",
    events: "Events",
    labels: "Labels",
    containers: "Containers",
    "deployment-detail": "Details",
    logs: "Logs",
    terminal: "Terminal",
    "aggregate-logs": "Aggregate Logs",
    service: "Endpoints",
    ingress: "Rules",
    configmap: "Data",
    secret: "Data",
    node: "Conditions",
    drain: "Drain",
    rules: "Rules",
    binding: "Binding",
    serviceaccount: "Token & Secrets",
    "sc-parameters": "Parameters",
    "csi-capabilities": "Capabilities",
    crd: "CRD",
    "crd-schema": "Schema",
    metrics: "Metrics",
    netpol: "Rules",
    endpointslice: "Addresses",
    resourcequota: "Usage",
    limitrange: "Limits",
    hpa: "Scaling",
    pdb: "Budget",
    webhooks: "Webhooks",
    conditions: "Conditions",
    metadata: "Metadata",
    drift: "Drift",
    related: "Related",
  };

  let {
    obj = $bindable(),
    descriptor,
    ctxName,
    gvr,
    namespace,
    name,
    onrefresh,
    onupdate,
    onopenowner,
    onopenresource,
  }: {
    obj: Record<string, KubernetesResource>;
    descriptor: DescriptorDef;
    ctxName: string;
    gvr: string;
    namespace: string;
    name: string;
    onrefresh: () => void;
    onupdate?: (updated: Record<string, KubernetesResource>) => void;
    onopenowner?: (ref: ControllerRef, namespace: string) => void;
    onopenresource?: (gvr: string, namespace: string, name: string) => void;
  } = $props();

  const splittablePanels = new Set<string>(["logs", "terminal", "aggregate-logs", "yaml"]);

  function openInBottomPanel(panel: string) {
    bottomPanelStore.addTab({
      kind: panel as PanelKind,
      resourceKind: descriptor.kind ?? gvr.split(".").at(-1) ?? "",
      resourceName: name,
      ctxName,
      gvr,
      namespace,
      name,
      obj,
    });
  }

  const foldedIntoOverview = new Set(["labels", "containers", "metadata"]);
  const visiblePanels = $derived.by(() => {
    const all = descriptor.detailPanels.filter((p) => panelComponents.has(p) && !foldedIntoOverview.has(p));
    if (getLastAppliedConfig(obj)) return all;
    return all.filter((p) => p !== "drift");
  });
  const pluginTabs = $derived(slotRegistry.getDetailTabs(gvr));
  let activePanel = $state("");
  let panelContainer = $state("");

  function openContainer(panel: "logs" | "terminal", container: string) {
    panelContainer = container;
    activePanel = panel;
  }
  $effect(() => {
    const allPanels = [...visiblePanels, ...pluginTabs.map((t) => t.id)];
    if (allPanels.length > 0 && !allPanels.includes(activePanel)) {
      activePanel = allPanels[0];
    }
  });

  const basePluginURL = $derived(streamingStore.config ? streamingStore.pluginsBaseUrl() : null);

  function makePluginCtx(tab: import("$lib/plugins/slots.svelte.js").RegisteredDetailTab) {
    const ns = clusterStore.getSelectedNamespaces(ctxName)[0] ?? namespace;
    const manifest = {
      schemaVersion: 1 as const,
      name: tab.pluginName,
      version: "",
      displayName: "",
      minHostVersion: "",
      permissions: {
        resources: tab.perms.resources?.map((p) => ({
          group: p.group,
          version: p.version,
          resource: p.resource,
          verbs: p.verbs as string[],
        })),
        logs: tab.perms.logs || undefined,
        exec: tab.perms.exec || undefined,
        storage: tab.perms.storage || undefined,
        events: tab.perms.events || undefined,
      },
    };
    return createPluginContext(manifest as unknown as PluginManifest, {
      clusterName: ctxName,
      clusterVersion: "",
      namespace: ns,
      listResources: (g, n) => ListResources(ctxName, g, n ?? ""),
      getResource: (g, n, name) => GetResource(ctxName, g, n, name),
    });
  }

  const uid = $derived<string>(obj.metadata?.uid ?? "");
</script>

<div class="flex flex-col h-full overflow-hidden">
  <!-- Actions toolbar -->
  {#if descriptor.actions.length > 0}
    <ActionsToolbar {obj} {ctxName} {gvr} {namespace} {name} actions={descriptor.actions} {onrefresh} />
  {/if}

  <!-- Validation warnings -->
  <ValidationWarningBanner {obj} />

  <!-- Pod failure diagnosis -->
  {#if gvr === 'core.v1.pods'}
    <PodDiagnosisBanner {obj} {ctxName} {namespace} {uid} setActivePanel={(p) => (activePanel = p)} />
  {/if}

  <!-- Panel tab bar -->
  <div class="flex items-center border-b border-border bg-surface shrink-0 overflow-x-auto">
    {#each visiblePanels as panel}
      <div class="flex items-center border-b-2 {activePanel === panel ? 'border-accent' : 'border-transparent'}">
        <button
          type="button"
          onclick={() => activePanel = panel}
          class="px-4 py-2 text-xs font-medium whitespace-nowrap transition-colors
            {activePanel === panel
              ? 'text-accent'
              : 'text-muted hover:text-fg hover:bg-surface-hover'}"
        >
          {panelLabels[panel] ?? panel}
        </button>
        {#if splittablePanels.has(panel)}
          <button
            type="button"
            onclick={(e) => { e.stopPropagation(); openInBottomPanel(panel) }}
            class="p-1 mr-1 rounded text-muted hover:text-fg hover:bg-surface-hover transition-colors"
            aria-label="Open in bottom panel"
            title="Open in bottom panel"
          >
            <ExternalLink size={11} />
          </button>
        {/if}
      </div>
    {/each}
    {#each pluginTabs as pt}
      <button
        type="button"
        onclick={() => activePanel = pt.id}
        class="px-4 py-2 text-xs font-medium whitespace-nowrap transition-colors border-b-2
          {activePanel === pt.id
            ? 'border-accent text-accent'
            : 'border-transparent text-muted hover:text-fg hover:bg-surface-hover'}"
      >
        {pt.label}
      </button>
    {/each}
  </div>

  <!-- Active panel -->
  <div class="flex-1 overflow-hidden">
    {#each pluginTabs as pt}
      {#if activePanel === pt.id}
        {#if basePluginURL}
          {#await loadPluginComponent(pt.pluginName, pt.component, basePluginURL) then Cmp}
            {#if Cmp}
              {@const ptCtx = makePluginCtx(pt)}
              <Cmp resource={obj} ctx={ptCtx} />
            {:else}
              <div class="flex items-center justify-center h-full text-sm text-muted">Plugin component failed to load</div>
            {/if}
          {/await}
        {:else}
          <div class="h-full bg-surface animate-pulse rounded"></div>
        {/if}
      {/if}
    {/each}
    {#each visiblePanels as panel}
      {#if activePanel === panel}
        {@const PanelCmp = panelComponents.get(panel) as PanelComponent}
        {#if panel === 'overview'}
          <PanelCmp
            {obj}
            onupdate={(updated: Record<string, unknown>) => { obj = updated; onupdate?.(updated) }}
            {descriptor}
            {gvr}
            {ctxName}
            {namespace}
            {name}
            {onopenowner}
            {onopenresource}
            onopencontainer={openContainer}
          />
        {:else if panel === 'yaml'}
          {#key uid}
            <PanelCmp
              {obj}
              onupdate={(updated: Record<string, unknown>) => { obj = updated; onupdate?.(updated) }}
              {ctxName}
              {gvr}
              {namespace}
              {name}
              kind={descriptor.kind ?? ''}
              {onrefresh}
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
          {/key}
        {:else if panel === 'events'}
          <PanelCmp {ctxName} {namespace} {uid} {onopenresource} />
        {:else if panel === 'labels'}
          <div class="overflow-auto h-full">
            <PanelCmp
              {obj}
              onupdate={(updated: Record<string, unknown>) => { obj = updated; onupdate?.(updated) }}
              {ctxName}
              {gvr}
              {namespace}
              {name}
            />
          </div>
        {:else if panel === 'containers'}
          <div class="overflow-auto h-full"><PanelCmp {obj} {ctxName} {onopenresource} onopencontainer={openContainer} /></div>
        {:else if panel === 'deployment-detail'}
          <div class="overflow-auto h-full"><PanelCmp {obj} /></div>
        {:else if panel === 'logs' || panel === 'terminal' || panel === 'aggregate-logs'}
          <PanelCmp {obj} {ctxName} {namespace} {name} initialContainer={panelContainer} />
        {:else if panel === 'service'}
          <PanelCmp {obj} {ctxName} />
        {:else if panel === 'ingress' || panel === 'configmap' || panel === 'secret' || panel === 'node' || panel === 'rules'}
          <div class="overflow-auto h-full"><PanelCmp {obj} /></div>
        {:else if panel === 'serviceaccount' || panel === 'binding'}
          <div class="overflow-auto h-full"><PanelCmp {obj} {ctxName} /></div>
        {:else if panel === 'sc-parameters'}
          <div class="overflow-auto h-full"><PanelCmp {obj} /></div>
        {:else if panel === 'csi-capabilities'}
          <div class="overflow-auto h-full"><PanelCmp {obj} {ctxName} /></div>
        {:else if panel === 'crd'}
          <div class="overflow-auto h-full"><PanelCmp {obj} {ctxName} /></div>
        {:else if panel === 'crd-schema'}
          <div class="h-full"><PanelCmp {obj} /></div>
        {:else if panel === 'metrics'}
          <PanelCmp {obj} {ctxName} {gvr} {namespace} {name} />
        {:else if panel === 'resourcequota' || panel === 'limitrange' || panel === 'pdb'}
          <div class="overflow-auto h-full"><PanelCmp {obj} /></div>
        {:else if panel === 'endpointslice'}
          <div class="overflow-auto h-full"><PanelCmp {obj} {ctxName} /></div>
        {:else if panel === 'netpol' || panel === 'webhooks' || panel === 'conditions' || panel === 'metadata'}
          <div class="overflow-auto h-full"><PanelCmp {obj} /></div>
        {:else if panel === 'hpa'}
          <div class="overflow-auto h-full"><PanelCmp {obj} {ctxName} /></div>
        {:else if panel === 'related'}
          <div class="overflow-auto h-full"><PanelCmp {obj} contextName={ctxName} {onopenresource} /></div>
        {/if}
      {/if}
    {/each}
  </div>
</div>
