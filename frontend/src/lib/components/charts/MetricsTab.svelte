<script lang="ts">
  import {untrack} from "svelte";
  import {
    GetCapabilities,
    GetResourceMetrics,
    GetPluginMetrics,
  } from "$api/github.com/Vilsol/klados/internal/services/metricsservice.js";
  import type {MetricsCapability, MetricsResponse, TimeSeries, TimeSeriesPoint} from "./types";
  import MetricsChart from "./MetricsChart.svelte";
  import TimeRangeSelector from "./TimeRangeSelector.svelte";
  import type {KubernetesResource} from "$lib/types";

  interface Props {
    obj: KubernetesResource;
    ctxName: string;
    gvr: string;
    namespace: string;
    name: string;
  }

  let {ctxName, gvr, namespace, name}: Props = $props();

  let capability: MetricsCapability | null = $state(null);
  let capabilityLoading: boolean = $state(true);
  let capabilityError: string | null = $state(null);
  let rangeMinutes: number = $state(15);
  let forceZero: boolean = $state(true);
  let zoomRange: {min: number; max: number} | null = $state(null);
  let response: MetricsResponse | null = $state(null);
  let fetchError: string | null = $state(null);
  // Rolling data accumulation for metrics-server (no Prometheus history)
  let rollingData: Map<string, TimeSeriesPoint[]> = $state(new Map());
  // Plugin metric results keyed by plugin name
  let pluginMetrics: Record<string, import("./types").MetricResult[]> = $state({});

  // Fetch capabilities once on mount
  $effect(() => {
    capabilityLoading = true;
    capabilityError = null;
    GetCapabilities(ctxName)
      .then((cap) => {
        capability = cap as unknown as MetricsCapability;
      })
      .catch((err: unknown) => {
        capabilityError = err instanceof Error ? err.message : String(err);
      })
      .finally(() => {
        capabilityLoading = false;
      });
  });

  // Reset data when the target resource changes so stale series don't persist
  $effect(() => {
    void [ctxName, gvr, namespace, name];
    untrack(() => {
      response = null;
      rollingData = new Map();
      fetchError = null;
      zoomRange = null;
      pluginMetrics = {};
    });
  });

  // Polling effect — tracks ctxName, gvr, namespace, name, rangeMinutes
  // Reads rollingData via untrack to avoid infinite loop
  $effect(() => {
    if (!capability) {
      return;
    }
    if (!(capability.hasMetricsServer || capability.hasPrometheus)) {
      return;
    }

    const interval = rangeMinutes <= 60 ? 15_000 : 60_000;

    async function fetchMetrics() {
      try {
        const res = await GetResourceMetrics(ctxName, gvr, namespace, name, rangeMinutes);
        fetchError = null;
        if (!res) {
          return;
        }

        const metricsRes = res as unknown as MetricsResponse;

        if (capability?.hasPrometheus) {
          response = metricsRes;
        } else {
          // Accumulate rolling data for metrics-server live mode
          untrack(() => {
            const nextMap = new Map(rollingData);
            for (const metric of metricsRes.metrics) {
              for (const series of metric.series) {
                const key = `${metric.name}:${series.labels.container ?? series.labels.pod ?? ""}`;
                const existing = nextMap.get(key) ?? [];
                const latest = series.points.at(-1);
                if (latest) {
                  existing.push(latest);
                  // Keep last 60 points (~15min at 15s interval)
                  if (existing.length > 60) {
                    existing.splice(0, existing.length - 60);
                  }
                }
                nextMap.set(key, existing);
              }
            }
            rollingData = nextMap;
            response = metricsRes;
          });
        }
      } catch (err: unknown) {
        fetchError = err instanceof Error ? err.message : String(err);
      }
    }

    fetchMetrics();
    const id = setInterval(fetchMetrics, interval);
    return () => clearInterval(id);
  });

  // Poll plugin metrics (Prometheus-only)
  $effect(() => {
    if (!capability?.hasPrometheus) {
      return;
    }

    const interval = rangeMinutes <= 60 ? 15_000 : 60_000;

    async function fetchPluginMetrics() {
      try {
        const res = await GetPluginMetrics(ctxName, gvr, namespace, name, rangeMinutes);
        if (res) {
          pluginMetrics = res as unknown as Record<string, import("./types").MetricResult[]>;
        } else {
          pluginMetrics = {};
        }
      } catch {
        // Plugin metrics are best-effort — don't show errors
      }
    }

    fetchPluginMetrics();
    const id = setInterval(fetchPluginMetrics, interval);
    return () => clearInterval(id);
  });

  function getSeriesForMetric(metricName: string): TimeSeries[] {
    if (!response) {
      return [];
    }

    if (!capability?.hasPrometheus && rollingData.size > 0) {
      // Build series from rolling data for this metric
      const containerMap = new Map<string, TimeSeriesPoint[]>();
      for (const [key, pts] of rollingData) {
        if (key.startsWith(`${metricName}:`)) {
          const label = key.slice(metricName.length + 1);
          containerMap.set(label, pts);
        }
      }
      return Array.from(containerMap.entries()).map(([container, points]) => ({
        labels: {container},
        points,
      }));
    }

    const metric = response.metrics.find((m) => m.name === metricName);
    const all = metric?.series ?? [];
    // Drop series where every label value is empty (e.g. Prometheus container="" artifact)
    return all.filter((s) => Object.values(s.labels).some((v) => v));
  }

  function getContainerNames(): string[] {
    if (!response) {
      return [];
    }
    const names = new Set<string>();
    for (const metric of response.metrics) {
      for (const s of metric.series) {
        const c = s.labels.container ?? s.labels.pod;
        if (c) {
          names.add(c);
        }
      }
    }
    return Array.from(names);
  }

  function getContainerSeries(container: string, metricName: string): TimeSeries[] {
    return getSeriesForMetric(metricName).filter((s) => (s.labels.container ?? s.labels.pod) === container);
  }
</script>

{#if capabilityLoading}
  <div class="flex items-center justify-center h-full text-xs text-muted">Loading metrics...</div>
{:else if capabilityError}
  <div class="flex flex-col items-center justify-center h-full gap-2 text-xs">
    <span class="text-destructive font-medium">Failed to load metrics capabilities</span>
    <span class="text-muted font-mono">{capabilityError}</span>
  </div>
{:else if !capability || (!(capability.hasMetricsServer || capability.hasPrometheus))}
  <div class="flex flex-col items-center justify-center h-full gap-1 text-xs text-muted">
    <span>No metrics sources available</span>
    <span>Install metrics-server or configure a Prometheus endpoint to enable metrics.</span>
  </div>
{:else}
  {@const cpuMetric = response?.metrics.find((m) => m.unit === 'cores' || m.name.toLowerCase().includes('cpu'))}
  {@const memMetric = response?.metrics.find((m) => m.unit === 'bytes' || m.name.toLowerCase().includes('mem'))}
  {@const cpuName = cpuMetric?.name ?? 'CPU Usage'}
  {@const memName = memMetric?.name ?? 'Memory Usage'}
  {@const containers = getContainerNames()}

  <div class="flex flex-col gap-4 p-4 overflow-auto h-full">
    <div class="flex items-center justify-between">
      <div class="flex items-center gap-2">
        <TimeRangeSelector value={rangeMinutes} hasPrometheus={capability.hasPrometheus} onchange={(v) => (rangeMinutes = v)} />
        <button
          type="button"
          class="px-2 py-0.5 text-xs rounded transition-colors {forceZero
            ? 'bg-accent text-white'
            : 'bg-surface text-muted hover:bg-surface-hover hover:text-fg'}"
          onclick={() => (forceZero = !forceZero)}
        >
          min: {forceZero ? '0' : 'auto'}
        </button>
      </div>
      <span class="text-xs text-muted"> {capability.hasPrometheus ? 'prometheus' : 'metrics-server (live only)'} </span>
    </div>

    {#if fetchError}
      <div class="flex items-center gap-2 text-xs px-2 py-1 rounded bg-destructive/10 text-destructive">
        <span>Failed to fetch metrics:</span>
        <span class="font-mono">{fetchError}</span>
      </div>
    {/if}

    <MetricsChart
      title={cpuName}
      unit="cores"
      series={getSeriesForMetric(cpuName)}
      thresholds={response?.thresholds?.filter((t) => t.label.toLowerCase().includes('cpu')) ?? []}
      annotations={response?.annotations ?? []}
      loading={!(response || fetchError)}
      {forceZero}
      {zoomRange}
      onzoom={(r) => (zoomRange = r)}
    />

    <MetricsChart
      title={memName}
      unit="bytes"
      series={getSeriesForMetric(memName)}
      thresholds={response?.thresholds?.filter((t) => t.label.toLowerCase().includes('memory') || t.label.toLowerCase().includes('mem')) ?? []}
      annotations={response?.annotations ?? []}
      loading={!(response || fetchError)}
      {forceZero}
      {zoomRange}
      onzoom={(r) => (zoomRange = r)}
    />

    {#if containers.length > 0}
      <div class="flex flex-col gap-3">
        <div class="text-sm font-medium text-fg">Per Container</div>
        {#each containers as container}
          <div class="flex flex-col gap-2">
            <div class="text-xs text-muted font-mono">{container}</div>
            <div class="grid grid-cols-2 gap-2">
              <MetricsChart
                title="CPU"
                unit="cores"
                series={getContainerSeries(container, cpuName)}
                thresholds={response?.thresholds?.filter((t) => t.label.toLowerCase().includes('cpu') && t.label.includes(`:${container}`)) ?? []}
                annotations={response?.annotations ?? []}
                height={120}
                {forceZero}
                {zoomRange}
                onzoom={(r) => (zoomRange = r)}
              />
              <MetricsChart
                title="Memory"
                unit="bytes"
                series={getContainerSeries(container, memName)}
                thresholds={response?.thresholds?.filter((t) => (t.label.toLowerCase().includes('memory') || t.label.toLowerCase().includes('mem')) && t.label.includes(`:${container}`)) ?? []}
                annotations={response?.annotations ?? []}
                height={120}
                {forceZero}
                {zoomRange}
                onzoom={(r) => (zoomRange = r)}
              />
            </div>
          </div>
        {/each}
      </div>
    {/if}

    {#if Object.keys(pluginMetrics).length > 0}
      {#each Object.entries(pluginMetrics) as [ pluginName, results ]}
        <div class="flex flex-col gap-2">
          <div class="text-sm font-medium text-fg">{pluginName}</div>
          {#each results as result}
            <MetricsChart
              title={result.name}
              unit={result.unit}
              series={result.series ?? []}
              loading={false}
              {forceZero}
              {zoomRange}
              onzoom={(r) => (zoomRange = r)}
            />
          {/each}
        </div>
      {/each}
    {/if}
  </div>
{/if}
