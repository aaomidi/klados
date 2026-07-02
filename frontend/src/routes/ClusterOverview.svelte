<script lang="ts">
  import {untrack} from "svelte";
  import {Events} from "@wailsio/runtime";
  import {onDestroy} from "svelte";
  import {clusterStore} from "$lib/stores/cluster.svelte";
  import {GetCapabilities, GetNamespaceMetrics} from "$api/github.com/Vilsol/klados/internal/services/metricsservice.js";
  import {GetClusterHealth} from "$api/github.com/Vilsol/klados/internal/services/appservice.js";
  import MetricsChart from "$lib/components/charts/MetricsChart.svelte";
  import TimeRangeSelector from "$lib/components/charts/TimeRangeSelector.svelte";
  import {Combobox} from "@klados/ui";
  import type {MetricsCapability, MetricsResponse, TimeSeries} from "$lib/components/charts/types";

  let {params = {}}: {params?: Record<string, string>} = $props();

  let ctxName = $derived(params.ctx ?? "unknown");

  $effect(() => {
    if (ctxName) {
      clusterStore.setActiveContext(ctxName);
    }
  });

  let namespaces = $derived(clusterStore.getNamespaces(ctxName));
  let selectedNamespace = $state("");

  $effect(() => {
    const ns = namespaces;
    untrack(() => {
      if (selectedNamespace === "" && ns.length > 0) {
        selectedNamespace = ns[0];
      }
    });
  });

  let capability: MetricsCapability | null = $state(null);
  let rangeMinutes: number = $state(15);
  let response: MetricsResponse | null = $state(null);
  let zoomRange: {min: number; max: number} | null = $state(null);
  let fetchError: string | null = $state(null);

  $effect(() => {
    GetCapabilities(ctxName)
      .then((cap) => {
        capability = cap as unknown as MetricsCapability;
      })
      .catch(() => {
        /* empty */
      });
  });

  $effect(() => {
    const ns = selectedNamespace;
    void [ctxName, ns];
    untrack(() => {
      response = null;
      fetchError = null;
      zoomRange = null;
    });
  });

  $effect(() => {
    const ns = selectedNamespace;
    if (!(ns && capability?.hasPrometheus)) {
      return;
    }

    const interval = rangeMinutes <= 60 ? 15_000 : 60_000;

    async function fetchMetrics() {
      try {
        const res = await GetNamespaceMetrics(ctxName, ns, rangeMinutes);
        fetchError = null;
        if (res) {
          response = res as unknown as MetricsResponse;
        }
      } catch (err: unknown) {
        fetchError = err instanceof Error ? err.message : String(err);
      }
    }

    fetchMetrics();
    const id = setInterval(fetchMetrics, interval);
    return () => clearInterval(id);
  });

  function getSeriesByUnit(unit: string): TimeSeries[] {
    if (!response) {
      return [];
    }
    const metric = response.metrics.find((m) => m.unit === unit || m.name.toLowerCase().includes(unit === "cores" ? "cpu" : "mem"));
    return metric?.series ?? [];
  }

  // --- Cluster health ---

  const HealthOK = 0;
  const HealthDegraded = 1;
  const HealthUnknown = 2;

  interface ComponentHealth {
    name: string;
    status: number;
    message: string;
  }
  interface ClusterHealth {
    apiServer: {livez: number; readyz: number; healthz: number};
    components: ComponentHealth[];
    nodes: {total: number; ready: number; notReady: number; schedulingDisabled: number; permissionDenied: boolean};
    checkedAt: string;
  }

  let health = $state<ClusterHealth | null>(null);

  function statusLabel(s: number): string {
    if (s === HealthOK) {
      return "OK";
    }
    if (s === HealthDegraded) {
      return "Degraded";
    }
    return "Unknown";
  }

  function statusClass(s: number): string {
    if (s === HealthOK) {
      return "bg-green-500/20 text-green-400 border-green-500/40";
    }
    if (s === HealthDegraded) {
      return "bg-red-500/20 text-red-400 border-red-500/40";
    }
    return "bg-surface text-muted border-border";
  }

  let healthUnsub: (() => void) | null = null;

  $effect(() => {
    healthUnsub?.();
    if (!ctxName) {
      return;
    }
    (async () => {
      try {
        const h = await GetClusterHealth(ctxName);
        if (h) {
          health = h as unknown as ClusterHealth;
        }
      } catch {
        /* empty */
      }
    })();
    healthUnsub = Events.On(`cluster:${ctxName}:health`, (wailsEvent: {data?: ClusterHealth}) => {
      health = (wailsEvent.data ?? wailsEvent) as ClusterHealth;
    });
    return () => {
      healthUnsub?.();
      healthUnsub = null;
    };
  });

  onDestroy(() => healthUnsub?.());
</script>

<div class="p-6 flex flex-col gap-6">
  <div class="flex items-center justify-between">
    <h1 class="text-2xl font-semibold">Cluster: {ctxName}</h1>

    {#if namespaces.length > 0}
      <div class="w-48">
        <Combobox
          bind:value={selectedNamespace}
          options={namespaces.map((ns) => ({ value: ns, label: ns }))}
          placeholder="Select namespace"
        />
      </div>
    {/if}
  </div>

  {#if health}
    <!-- API Server probes -->
    <section class="flex flex-col gap-3">
      <h2 class="text-xs font-semibold uppercase tracking-wider text-muted">API Server</h2>
      <div class="flex gap-3 flex-wrap">
        {#each [['livez', health.apiServer.livez], ['readyz', health.apiServer.readyz], ['healthz', health.apiServer.healthz]] as [ probe, status ]}
          <div class="flex items-center gap-2 px-3 py-1.5 rounded border text-xs {statusClass(status as number)}">
            <span class="font-mono">/{probe}</span>
            <span class="font-semibold">{statusLabel(status as number)}</span>
          </div>
        {/each}
      </div>
    </section>

    <!-- Component statuses -->
    <section class="flex flex-col gap-3">
      <h2 class="text-xs font-semibold uppercase tracking-wider text-muted">Component Statuses</h2>
      {#if health.components.length === 0}
        <p class="text-xs text-muted italic">Not exposed by this cluster</p>
      {:else}
        <div class="border border-border rounded overflow-hidden">
          <table class="w-full text-sm">
            <thead>
              <tr class="bg-surface border-b border-border">
                <th class="text-left px-4 py-2 text-xs font-semibold text-muted uppercase tracking-wider">Component</th>
                <th class="text-left px-4 py-2 text-xs font-semibold text-muted uppercase tracking-wider">Status</th>
                <th class="text-left px-4 py-2 text-xs font-semibold text-muted uppercase tracking-wider">Message</th>
              </tr>
            </thead>
            <tbody>
              {#each health.components as comp}
                <tr class="border-b border-border last:border-0 hover:bg-surface-hover">
                  <td class="px-4 py-2 font-mono text-xs">{comp.name}</td>
                  <td class="px-4 py-2">
                    <span class="px-2 py-0.5 rounded border text-xs {statusClass(comp.status)}">
                      {comp.status === HealthUnknown ? 'Not exposed' : statusLabel(comp.status)}
                    </span>
                  </td>
                  <td class="px-4 py-2 text-xs text-muted">{comp.message || '—'}</td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}
    </section>

    <!-- Node summary -->
    <section class="flex flex-col gap-3">
      <h2 class="text-xs font-semibold uppercase tracking-wider text-muted">Nodes</h2>
      {#if health.nodes.permissionDenied}
        <p class="text-xs text-muted italic">Insufficient permissions to list nodes</p>
      {:else}
        <div class="flex gap-3 flex-wrap">
          <div class="flex flex-col items-center px-5 py-3 rounded border border-border bg-surface gap-0.5">
            <span class="text-xl font-semibold">{health.nodes.total}</span>
            <span class="text-xs text-muted">Total</span>
          </div>
          <div class="flex flex-col items-center px-5 py-3 rounded border border-green-500/40 bg-green-500/10 gap-0.5">
            <span class="text-xl font-semibold text-green-400">{health.nodes.ready}</span>
            <span class="text-xs text-muted">Ready</span>
          </div>
          {#if health.nodes.notReady > 0}
            <div class="flex flex-col items-center px-5 py-3 rounded border border-red-500/40 bg-red-500/10 gap-0.5">
              <span class="text-xl font-semibold text-red-400">{health.nodes.notReady}</span>
              <span class="text-xs text-muted">Not Ready</span>
            </div>
          {/if}
          {#if health.nodes.schedulingDisabled > 0}
            <div class="flex flex-col items-center px-5 py-3 rounded border border-amber-500/40 bg-amber-500/10 gap-0.5">
              <span class="text-xl font-semibold text-amber-400">{health.nodes.schedulingDisabled}</span>
              <span class="text-xs text-muted">Scheduling Disabled</span>
            </div>
          {/if}
        </div>
      {/if}
    </section>
  {/if}

  {#if selectedNamespace}
    {#if !capability?.hasPrometheus}
      <p class="text-xs text-muted">Configure a Prometheus endpoint to view namespace metrics.</p>
    {:else}
      <div class="flex items-center justify-between">
        <span class="text-sm font-medium">Namespace: {selectedNamespace}</span>
        <div class="flex items-center gap-2">
          <TimeRangeSelector value={rangeMinutes} hasPrometheus={true} onchange={(v) => (rangeMinutes = v)} />
          <span class="text-xs text-muted">prometheus</span>
        </div>
      </div>

      {#if fetchError}
        <div class="text-xs text-destructive font-mono">{fetchError}</div>
      {/if}

      <MetricsChart
        title="CPU Usage"
        unit="cores"
        series={getSeriesByUnit('cores')}
        loading={!(response || fetchError)}
        {zoomRange}
        onzoom={(r) => (zoomRange = r)}
      />
      <MetricsChart
        title="Memory Usage"
        unit="bytes"
        series={getSeriesByUnit('bytes')}
        loading={!(response || fetchError)}
        {zoomRange}
        onzoom={(r) => (zoomRange = r)}
      />
    {/if}
  {:else}
    <p class="text-muted text-sm">No namespaces found.</p>
  {/if}
</div>
