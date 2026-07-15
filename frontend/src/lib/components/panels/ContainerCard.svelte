<script lang="ts">
  import type {Snippet} from "svelte";
  import {formatAge} from "$lib/utils/age";
  import {CopyableValue, StatusBadge} from "@klados/ui";
  import PortButton from "$lib/components/PortButton.svelte";
  import {
    containerStateInfo,
    lastExit,
    decodeExitCode,
    probeSummaries,
    envSource,
    envFromSources,
    mountSource,
    resourceSummary,
    type RefLink,
  } from "$lib/kubernetes/containers";
  import type {KubernetesResource} from "$lib/types";

  let {
    container,
    status,
    volumes = [],
    namespace = "",
    compact = false,
    onopenresource,
    onopencontainer,
    onforwardport,
    portBadge,
  }: {
    container: KubernetesResource;
    status?: KubernetesResource;
    volumes?: KubernetesResource[];
    namespace?: string;
    compact?: boolean;
    onopenresource?: (gvr: string, namespace: string, name: string) => void;
    onopencontainer?: (panel: "logs" | "terminal", container: string) => void;
    onforwardport?: (port: number) => void;
    portBadge?: Snippet<[KubernetesResource]>;
  } = $props();

  const info = $derived(containerStateInfo(status));
  const lx = $derived(lastExit(status));
  const probes = $derived(probeSummaries(container));
  const resources = $derived(resourceSummary(container.resources));
  const envFrom = $derived(envFromSources(container));
  const envCount = $derived((container.env?.length ?? 0) + envFrom.length);
  const mounts = $derived<KubernetesResource[]>(container.volumeMounts ?? []);

  let envOpen = $state(false);
  let mountsOpen = $state(false);

  function openRef(ref: RefLink) {
    if (ref.gvr && ref.name) onopenresource?.(ref.gvr, namespace, ref.name);
  }

  function absoluteTime(ts?: string): string {
    return ts ? new Date(ts).toLocaleString() : "";
  }
</script>

<div class="bg-bg border border-border rounded-lg p-3">
  <!-- Header -->
  <div class="flex items-center justify-between gap-2">
    <CopyableValue value={container.name} class="text-sm font-medium min-w-0" />
    <div class="flex items-center gap-1.5 shrink-0">
      {#if info.kind === 'running' && info.since}
        <span class="text-xs text-muted tabular-nums" title={absoluteTime(info.since)}>up {formatAge(info.since)}</span>
      {/if}
      {#if status?.restartCount > 0}
        <span class="text-xs px-2 py-0.5 rounded-full bg-yellow-500/15 text-yellow-600 dark:text-yellow-400">
          {status.restartCount}
          restart{status.restartCount === 1 ? '' : 's'}
        </span>
      {/if}
      <StatusBadge
        status={Boolean(status?.ready) || (info.kind === 'terminated' && info.exitCode === 0)}
        mode="pill"
      >
        {info.label}{info.kind === 'terminated' && info.exitCode !== undefined
          ? ` (exit ${info.exitCode} · ${decodeExitCode(info.exitCode, info.label)})`
          : ''}
      </StatusBadge>
      {#if onopencontainer}
        <button
          type="button"
          onclick={() => onopencontainer?.('logs', container.name)}
          class="text-xs px-2 py-0.5 rounded border border-border text-muted hover:text-fg hover:bg-surface-hover transition-colors"
        >
          Logs
        </button>
        {#if !compact && info.kind === 'running'}
          <button
            type="button"
            onclick={() => onopencontainer?.('terminal', container.name)}
            class="text-xs px-2 py-0.5 rounded border border-border text-muted hover:text-fg hover:bg-surface-hover transition-colors"
          >
            Shell
          </button>
        {/if}
      {/if}
    </div>
  </div>

  <CopyableValue value={container.image} class="text-xs font-mono text-muted break-all mt-0.5" />

  <!-- State message (waiting/terminated) -->
  {#if info.message}
    <p class="text-xs mt-1.5 {info.kind === 'waiting' ? 'text-amber-500' : 'text-destructive'}">
      {info.label}
      — {info.message}
    </p>
  {/if}

  <!-- Crash forensics -->
  {#if lx}
    <p class="text-xs mt-1.5 text-destructive" title={absoluteTime(lx.finishedAt)}>
      Last exit: {lx.reason} (exit {lx.exitCode} · {decodeExitCode(lx.exitCode, lx.reason)}){lx.finishedAt
        ? ` · ${formatAge(lx.finishedAt)} ago`
        : ''}
    </p>
  {/if}

  {#if !compact}
    <!-- Detail rows -->
    <div class="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 mt-2.5 items-baseline">
      {#if resources}
        <span class="text-xs text-muted">Resources</span>
        <span class="text-xs font-mono" title="request / limit">{resources}</span>
      {/if}
      {#if container.ports?.length}
        <span class="text-xs text-muted">Ports</span>
        <span class="flex flex-wrap gap-1">
          {#each container.ports as p}
            <span class="inline-flex items-center gap-1">
              <PortButton
                port={p.containerPort}
                hostPort={p.hostPort}
                protocol={p.protocol ?? 'TCP'}
                name={p.name}
                onclick={() => onforwardport?.(p.containerPort)}
              />
              {#if portBadge}{@render portBadge(p)}{/if}
            </span>
          {/each}
        </span>
      {/if}
      {#if probes.length > 0}
        <span class="text-xs text-muted">Probes</span>
        <span class="text-xs font-mono">
          {#each probes as probe, i}
            {#if i > 0}<span class="text-muted"> · </span>{/if}<span>{`${probe.kind} ${probe.text}`}</span>
          {/each}
        </span>
      {/if}
    </div>

    <!-- Env / Mounts toggles -->
    {#if envCount > 0 || mounts.length > 0}
      <div class="flex gap-4 mt-2">
        {#if envCount > 0}
          <button type="button" onclick={() => (envOpen = !envOpen)} class="text-xs text-accent hover:underline">
            {envOpen ? '▾' : '▸'}
            {envCount} env var{envCount === 1 ? '' : 's'}
          </button>
        {/if}
        {#if mounts.length > 0}
          <button type="button" onclick={() => (mountsOpen = !mountsOpen)} class="text-xs text-accent hover:underline">
            {mountsOpen ? '▾' : '▸'}
            {mounts.length} mount{mounts.length === 1 ? '' : 's'}
          </button>
        {/if}
      </div>
    {/if}

    {#if envOpen}
      <div class="mt-1.5 rounded-md border border-border divide-y divide-border/60 overflow-hidden">
        {#each envFrom as src}
          <div class="grid grid-cols-[minmax(8rem,30%)_1fr] gap-x-4 px-2.5 py-1 items-baseline">
            <span class="text-xs font-mono text-muted">envFrom</span>
            {#if src.gvr && src.name && onopenresource}
              <button type="button" onclick={() => openRef(src)} class="text-xs font-mono text-accent hover:underline text-left justify-self-start">
                {src.text}
              </button>
            {:else}
              <span class="text-xs font-mono text-muted">{src.text}</span>
            {/if}
          </div>
        {/each}
        {#each container.env ?? [] as e}
          {@const src = envSource(e)}
          <div class="grid grid-cols-[minmax(8rem,30%)_1fr] gap-x-4 px-2.5 py-1 items-baseline">
            <CopyableValue value={e.name} class="text-xs font-mono text-accent break-all" />
            {#if src}
              {#if src.gvr && src.name && onopenresource}
                <button type="button" onclick={() => openRef(src)} class="text-xs font-mono text-accent hover:underline text-left justify-self-start">
                  {src.text}
                </button>
              {:else}
                <span class="text-xs font-mono text-muted">{src.text}</span>
              {/if}
            {:else}
              <CopyableValue value={e.value ?? '—'} class="text-xs font-mono text-muted break-all" />
            {/if}
          </div>
        {/each}
      </div>
    {/if}

    {#if mountsOpen}
      <div class="mt-1.5 rounded-md border border-border divide-y divide-border/60 overflow-hidden">
        {#each mounts as m}
          {@const src = mountSource(m, volumes)}
          <div class="flex items-center gap-2 px-2.5 py-1 text-xs min-w-0">
            <CopyableValue value={m.mountPath} class="font-mono break-all min-w-0" />
            {#if m.subPath}
              <span class="font-mono text-muted shrink-0">/{m.subPath}</span>
            {/if}
            {#if m.readOnly}
              <span class="px-1.5 py-0.5 rounded bg-surface border border-border text-muted text-[10px] shrink-0">RO</span>
            {/if}
            <span class="ml-auto shrink-0 pl-4">
              {#if src.gvr && src.name && onopenresource}
                <button type="button" onclick={() => openRef(src)} class="font-mono text-accent hover:underline">
                  {src.text}
                </button>
              {:else}
                <span class="font-mono text-muted">{src.text}</span>
              {/if}
            </span>
          </div>
        {/each}
      </div>
    {/if}
  {/if}
</div>
