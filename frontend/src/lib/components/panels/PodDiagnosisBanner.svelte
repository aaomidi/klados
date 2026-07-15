<script lang="ts">
  import {GetEvents, StartWatch, StopWatch} from "../../../../bindings/github.com/Vilsol/klados/internal/services/resourceservice.js";
  import {Events as WailsEvents} from "@wailsio/runtime";
  import {formatAge} from "$lib/utils/age";
  import {classifySeverity, eventTimestamp} from "$lib/event/event-columns";
  import type {EventItem} from "$lib/event/event-types";
  import {podDiagnosis} from "$lib/kubernetes/containers";
  import type {KubernetesResource} from "$lib/types";

  let {
    obj,
    ctxName,
    namespace,
    uid,
    setActivePanel,
  }: {
    obj: KubernetesResource;
    ctxName: string;
    namespace: string;
    uid: string;
    setActivePanel: (panel: string) => void;
  } = $props();

  const diag = $derived(podDiagnosis(obj));

  let events = $state<EventItem[]>([]);

  function tsMs(e: EventItem): number {
    const t = Date.parse(eventTimestamp(e));
    return Number.isNaN(t) ? 0 : t;
  }

  const warnings = $derived(
    [...events]
      .filter((e) => classifySeverity(e) === "Warning")
      .sort((a, b) => tsMs(b) - tsMs(a))
      .slice(0, 5),
  );

  function upsert(ev: EventItem) {
    const idx = events.findIndex((e) => e.metadata?.uid === ev.metadata?.uid);
    if (idx >= 0) events[idx] = ev;
    else events.push(ev);
  }

  $effect(() => {
    const currentCtx = ctxName;
    const currentNs = namespace;
    const currentUid = uid;
    events = [];
    if (!currentUid) return;

    StartWatch(currentCtx, "core.v1.events", currentNs, "").catch(() => {});
    const unsub = WailsEvents.On(`watch:${currentCtx}:core.v1.events:${currentNs}`, (wailsEvent: unknown) => {
      const data = (wailsEvent as {data?: {type?: string; object?: EventItem}}).data;
      const ev = data?.object;
      if (!ev || ev.involvedObject?.uid !== currentUid) return;
      if (data?.type === "DELETED") {
        events = events.filter((e) => e.metadata?.uid !== ev.metadata?.uid);
      } else {
        upsert(ev);
      }
    });

    GetEvents(currentCtx, currentNs, currentUid)
      .then((listed) => {
        for (const item of (listed ?? []) as EventItem[]) {
          if (!events.some((e) => e.metadata?.uid === item.metadata?.uid)) events.push(item);
        }
      })
      .catch(() => {});

    return () => {
      unsub?.();
      StopWatch(currentCtx, "core.v1.events", currentNs).catch(() => {});
    };
  });

  function absoluteTime(ts: string): string {
    return ts ? new Date(ts).toLocaleString() : "";
  }
</script>

{#if diag.show}
  <div class="shrink-0 border-b border-destructive/40 bg-destructive/5 px-4 py-2.5 text-sm">
    <div class="flex items-center gap-2">
      <span class="font-medium text-destructive">Pod unhealthy</span>
      {#if diag.reason}
        <span class="px-2 py-0.5 rounded-full bg-destructive/15 text-destructive text-xs font-medium">{diag.reason}</span>
      {/if}
    </div>

    {#if diag.message}
      <p class="mt-1 text-xs text-fg/80 break-words">{diag.message}</p>
    {/if}

    {#if diag.unhealthy > 0}
      <button
        type="button"
        onclick={() => setActivePanel('overview')}
        class="mt-1 text-xs text-accent hover:underline"
      >
        {diag.unhealthy} of {diag.total} containers unhealthy — view details
      </button>
    {/if}

    {#if warnings.length > 0}
      <div class="mt-2 rounded-md border border-border divide-y divide-border/60 overflow-hidden">
        {#each warnings as ev (ev.metadata?.uid)}
          {@const ts = eventTimestamp(ev)}
          <div class="grid grid-cols-[auto_1fr_auto] gap-x-3 px-2.5 py-1 items-baseline">
            <span class="text-xs font-mono text-amber-500">{ev.reason ?? ''}</span>
            <span class="text-xs text-muted break-words">{ev.message ?? ''}</span>
            <span class="text-xs text-muted whitespace-nowrap tabular-nums" title={absoluteTime(ts)}>
              {ts ? formatAge(ts) : '—'}
            </span>
          </div>
        {/each}
        <button
          type="button"
          onclick={() => setActivePanel('events')}
          class="w-full text-left px-2.5 py-1 text-xs text-accent hover:underline hover:bg-surface-hover"
        >
          View all events
        </button>
      </div>
    {/if}
  </div>
{/if}
