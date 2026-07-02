<script lang="ts">
  import {GetEvents, StartWatch, StopWatch} from "$api/github.com/Vilsol/klados/internal/services/resourceservice.js";
  import {Events as WailsEvents} from "@wailsio/runtime";
  import {formatAge} from "$lib/utils/age";
  import EventTypeBadge from "$lib/event/EventTypeBadge.svelte";
  import {classifySeverity, eventTimestamp, eventFirstTimestamp} from "$lib/event/event-columns";
  import {extractMessageRefs, type MessageRef} from "$lib/event/event-refs";
  import type {EventItem, InvolvedObjectRef} from "$lib/event/event-types";
  import {clusterStore} from "$lib/stores/cluster.svelte";

  let {
    ctxName,
    namespace,
    uid,
    onopenresource,
  }: {
    ctxName: string;
    namespace: string; // "" for cluster-scoped resources (all-namespaces search)
    uid: string;
    onopenresource?: (gvr: string, namespace: string, name: string) => void;
  } = $props();

  let events = $state<EventItem[]>([]);
  let loading = $state(true);
  let error = $state<string | null>(null);
  let warningsOnly = $state(false);

  function tsMs(e: EventItem): number {
    const t = Date.parse(eventTimestamp(e));
    return Number.isNaN(t) ? 0 : t;
  }

  const sorted = $derived([...events].sort((a, b) => tsMs(b) - tsMs(a)));
  const warningCount = $derived(events.filter((e) => classifySeverity(e) === "Warning").length);
  const visible = $derived(warningsOnly ? sorted.filter((e) => classifySeverity(e) === "Warning") : sorted);

  function upsert(ev: EventItem) {
    const idx = events.findIndex((e) => e.metadata?.uid === ev.metadata?.uid);
    if (idx >= 0) {
      events[idx] = ev;
    } else {
      events.push(ev);
    }
  }

  $effect(() => {
    const currentCtx = ctxName;
    const currentNs = namespace;
    const currentUid = uid;

    events = [];
    loading = true;
    error = null;

    // The watch:{ctx}:core.v1.events:{ns} stream only emits while a watch is
    // running — demand-start our own (WatchManager refcounts + 30s grace).
    StartWatch(currentCtx, "core.v1.events", currentNs, "").catch(() => {});
    const unsub = WailsEvents.On(`watch:${currentCtx}:core.v1.events:${currentNs}`, (wailsEvent: unknown) => {
      const data = (wailsEvent as {data?: {type?: string; object?: EventItem}}).data;
      const obj = data?.object;
      if (!obj || obj.involvedObject?.uid !== currentUid) return;
      if (data?.type === "DELETED") {
        events = events.filter((e) => e.metadata?.uid !== obj.metadata?.uid);
      } else {
        upsert(obj);
      }
    });

    GetEvents(currentCtx, currentNs, currentUid)
      .then((listed) => {
        // watch deltas may have arrived while listing — those are newer, keep them
        for (const item of (listed ?? []) as EventItem[]) {
          if (!events.some((e) => e.metadata?.uid === item.metadata?.uid)) {
            events.push(item);
          }
        }
        error = null;
      })
      .catch((e: unknown) => {
        error = e instanceof Error ? e.message : String(e);
      })
      .finally(() => {
        loading = false;
      });

    return () => {
      unsub?.();
      StopWatch(currentCtx, "core.v1.events", currentNs).catch(() => {});
    };
  });

  function messageSegments(message: string): Array<{text: string; ref?: MessageRef}> {
    const refs = extractMessageRefs(message);
    if (refs.length === 0) return [{text: message}];
    const out: Array<{text: string; ref?: MessageRef}> = [];
    let pos = 0;
    for (const ref of refs) {
      if (ref.start > pos) out.push({text: message.slice(pos, ref.start)});
      out.push({text: ref.name, ref});
      pos = ref.end;
    }
    if (pos < message.length) out.push({text: message.slice(pos)});
    return out;
  }

  function openMessageRef(ref: MessageRef, ev: EventItem) {
    const ns = ref.clusterScoped ? "" : (ev.metadata?.namespace ?? namespace);
    onopenresource?.(ref.gvr, ns, ref.name);
  }

  function relatedGVR(related: Partial<InvolvedObjectRef>): string | undefined {
    if (!related.kind || !related.name) return undefined;
    return clusterStore.resolveOwnerGVR(related.apiVersion ?? "", related.kind);
  }

  function absoluteTime(ts: string): string {
    return ts ? new Date(ts).toLocaleString() : "";
  }
</script>

<div class="flex flex-col h-full overflow-auto">
  {#if loading}
    <div class="p-4 text-sm text-muted">Loading events...</div>
  {:else if error}
    <div class="p-4 text-sm text-destructive">{error}</div>
  {:else if events.length === 0}
    <div class="p-4 text-sm text-muted">No events found.</div>
  {:else}
    <div class="flex items-center gap-2 px-3 py-1.5 border-b border-border text-xs">
      <span class="text-muted">{events.length} event{events.length === 1 ? '' : 's'}</span>
      {#if warningCount > 0}
        <button
          class="px-2 py-0.5 rounded border transition-colors {warningsOnly ? 'border-amber-500 bg-amber-500/10 text-amber-500' : 'border-border text-muted hover:bg-surface-hover'}"
          onclick={() => (warningsOnly = !warningsOnly)}
        >
          Warnings {warningCount}
        </button>
      {/if}
    </div>
    <table class="w-full text-xs">
      <thead class="sticky top-0 bg-surface border-b border-border">
        <tr>
          <th class="text-left px-3 py-2 font-medium text-muted w-16">Type</th>
          <th class="text-left px-3 py-2 font-medium text-muted w-32">Reason</th>
          <th class="text-left px-3 py-2 font-medium text-muted">Message</th>
          <th class="text-left px-3 py-2 font-medium text-muted w-12">Count</th>
          <th class="text-left px-3 py-2 font-medium text-muted w-20">First seen</th>
          <th class="text-left px-3 py-2 font-medium text-muted w-20">Last seen</th>
          <th class="text-left px-3 py-2 font-medium text-muted w-28">Source</th>
        </tr>
      </thead>
      <tbody>
        {#each visible as ev (ev.metadata?.uid ?? `${ev.reason}/${ev.metadata?.name}`)}
          {@const message = ev.message ?? ''}
          {@const count = ev.count ?? 1}
          {@const firstTs = eventFirstTimestamp(ev)}
          {@const lastTs = eventTimestamp(ev)}
          {@const source = ev.source?.component ?? ev.reportingController ?? ''}
          {@const isWarning = classifySeverity(ev) === 'Warning'}
          {@const relGvr = ev.related ? relatedGVR(ev.related) : undefined}
          <tr class="border-b border-border hover:bg-surface-hover {isWarning ? 'bg-amber-500/5' : ''}">
            <td class="px-3 py-1.5">
              <EventTypeBadge severity={classifySeverity(ev)} />
            </td>
            <td class="px-3 py-1.5 font-mono {isWarning ? 'text-amber-500' : 'text-muted'}">{ev.reason ?? ''}</td>
            <td class="px-3 py-1.5 text-muted break-words">
              {#each messageSegments(message) as seg}
                {#if seg.ref}
                  <button
                    class="text-accent hover:underline"
                    onclick={() => openMessageRef(seg.ref!, ev)}
                  >{seg.text}</button>
                {:else}{seg.text}{/if}
              {/each}
              {#if ev.related?.name && relGvr}
                <button
                  class="ml-1 text-accent hover:underline"
                  onclick={() => onopenresource?.(relGvr, ev.related?.namespace ?? namespace, ev.related?.name ?? '')}
                >{ev.related.kind}/{ev.related.name}</button>
              {/if}
            </td>
            <td class="px-3 py-1.5 text-muted">{count}</td>
            <td class="px-3 py-1.5 text-muted whitespace-nowrap" title={absoluteTime(firstTs)}>{firstTs ? formatAge(firstTs) : '—'}</td>
            <td class="px-3 py-1.5 text-muted whitespace-nowrap" title={absoluteTime(lastTs)}>{lastTs ? formatAge(lastTs) : '—'}</td>
            <td class="px-3 py-1.5 text-muted">{source}</td>
          </tr>
        {/each}
      </tbody>
    </table>
  {/if}
</div>
