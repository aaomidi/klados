<script lang="ts">
  import {X} from "lucide-svelte";
  import {Combobox} from "@klados/ui";
  import {StartForward, ListForwards} from "$api/github.com/Vilsol/klados/internal/services/portforwardservice.js";
  import {TargetKind} from "$api/github.com/Vilsol/klados/internal/portforward/models.js";
  import {Browser} from "@wailsio/runtime";
  import {capabilitiesStore} from "$lib/stores/capabilities.svelte.js";
  import {notificationStore} from "$lib/stores/notification.svelte";
  import {unwrapError} from "$lib/utils/async.js";
  import {clusterStore} from "$lib/stores/cluster.svelte";
  import {getLogger} from "$lib/logger";

  const log = getLogger("portforward");

  let {
    onclose,
    oncreated,
    // Quick mode: all context pre-filled, only ask about local port
    prefillContext = "",
    prefillNamespace = "",
    prefillTargetKind = "",
    prefillTarget = "",
    prefillGVR = "",
    prefillRemotePort = 0,
  }: {
    onclose: () => void;
    oncreated?: (spec: unknown) => void;
    prefillContext?: string;
    prefillNamespace?: string;
    prefillTargetKind?: string;
    prefillTarget?: string;
    prefillGVR?: string;
    prefillRemotePort?: number;
  } = $props();

  const isQuickMode = $derived(Boolean(prefillTarget) && prefillRemotePort > 0);

  // Quick mode state
  let localPortMode = $state<"auto" | "custom">("auto");
  let customLocalPort = $state("");

  // Full mode state
  // svelte-ignore state_referenced_locally
  let targetKind = $state(prefillGVR ? "selector" : prefillTargetKind || "pod");
  // svelte-ignore state_referenced_locally
  let targetName = $state(prefillTarget);
  // svelte-ignore state_referenced_locally
  let targetGVR = $state(prefillGVR);
  let localPort = $state("");
  // svelte-ignore state_referenced_locally
  let remotePort = $state(prefillRemotePort > 0 ? String(prefillRemotePort) : "");
  // svelte-ignore state_referenced_locally
  let namespace = $state(prefillNamespace || (clusterStore.getSelectedNamespaces(clusterStore.activeContext ?? "")[0] ?? "default"));
  let submitting = $state(false);
  let openInBrowser = $state(true);

  // Poll ListForwards until the given forward reports active with a real local
  // port, then open it in the (system, on desktop) browser exactly once.
  async function openWhenActive(ctx: string, id: string) {
    for (let i = 0; i < 50; i++) {
      try {
        const forwards = (await ListForwards(ctx)) as Array<{id: string; status?: string; localPort?: number}>;
        const fw = forwards.find((f) => f.id === id);
        if (fw?.status === "active" && fw.localPort && fw.localPort > 0) {
          Browser.OpenURL(`http://localhost:${fw.localPort}`);
          return;
        }
        if (fw && (fw.status === "failed" || fw.status === "stopped")) {
          return;
        }
      } catch {
        // transient — keep polling
      }
      await new Promise((r) => setTimeout(r, 300));
    }
  }

  async function submit() {
    submitting = true;
    try {
      const ctx = isQuickMode ? prefillContext : (clusterStore.activeContext ?? "");
      const ns = isQuickMode ? prefillNamespace : namespace;
      const kind = isQuickMode ? prefillTargetKind : targetKind;
      const name = isQuickMode ? prefillTarget : targetName;
      const gvr = isQuickMode ? prefillGVR : targetGVR;
      const remote = isQuickMode ? prefillRemotePort : Number.parseInt(remotePort, 10);
      let local: number;
      if (isQuickMode) {
        local = localPortMode === "auto" ? 0 : Number.parseInt(customLocalPort, 10) || 0;
      } else {
        local = localPort ? Number.parseInt(localPort, 10) : 0;
      }

      if (!name || Number.isNaN(remote) || remote <= 0 || !ns) {
        return;
      }

      const spec = await StartForward(ctx, ns, kind as TargetKind, name, gvr, local, remote);
      log.info("Port forward started", {localPort: local, remotePort: remote});
      oncreated?.(spec);
      if (openInBrowser) {
        // Poll for the forward to go active (the tunnel establishes async and
        // gets its real local port then) and open the browser once. Polling
        // rather than listening avoids racing the `active` status event, which
        // can fire before a freshly-registered event subscription goes live.
        void openWhenActive(ctx, spec.id);
      }
      onclose();
    } catch (e: unknown) {
      notificationStore.error(unwrapError(e));
    } finally {
      submitting = false;
    }
  }
</script>

<!-- Backdrop -->
<div class="fixed inset-0 bg-black/50 z-40 flex items-center justify-center" role="dialog" aria-modal="true">
  <div class="bg-surface border border-border rounded-lg {isQuickMode ? 'w-80' : 'w-[26rem]'} shadow-xl z-50">
    <div class="flex items-center justify-between px-4 py-3 border-b border-border">
      <h2 class="text-sm font-semibold">{isQuickMode ? 'Forward Port' : 'Start Port Forward'}</h2>
      <button type="button" onclick={onclose} class="p-1 rounded hover:bg-surface-hover transition-colors"><X size={14} /></button>
    </div>

    <div class="p-4 flex flex-col gap-3">
      {#if capabilitiesStore.loaded && !capabilitiesStore.portForwarding}
        <p class="text-sm text-muted">
          Port forwarding isn't available on a hosted klados server — the tunnel would open on the
          server's loopback, not this machine. Use the desktop app, or run
          <code class="font-mono text-xs">kubectl port-forward</code> locally.
        </p>
      {:else if isQuickMode}
        <div class="bg-surface-hover border border-border rounded px-3 py-2">
          <p class="text-xs text-muted mb-0.5">Target</p>
          <p class="text-sm font-mono">{prefillTarget} <span class="text-muted">→ :{prefillRemotePort}</span></p>
        </div>

        <div class="flex flex-col gap-2">
          <p class="text-xs text-muted font-medium">Local port</p>
          <label class="flex items-center gap-2 text-sm cursor-pointer">
            <input type="radio" name="local-port-mode" value="auto" bind:group={localPortMode} class="accent-accent">
            Auto-assign
          </label>
          <label class="flex items-center gap-2 text-sm cursor-pointer">
            <input type="radio" name="local-port-mode" value="custom" bind:group={localPortMode} class="accent-accent">
            Custom:
            <input
              bind:value={customLocalPort}
              placeholder={String(prefillRemotePort)}
              disabled={localPortMode !== 'custom'}
              onclick={() => localPortMode = 'custom'}
              class="w-20 text-sm bg-surface border border-border rounded px-2 py-0.5 font-mono disabled:opacity-40"
            >
          </label>
        </div>
      {:else}
        <div class="flex flex-col gap-1">
          <!-- svelte-ignore a11y_label_has_associated_control -->
          <label class="text-xs text-muted">Target type</label>
          <Combobox
            bind:value={targetKind}
            options={[
              { value: 'pod', label: 'Pod (direct)' },
              { value: 'statefulpod', label: 'StatefulSet pod (stable name)' },
              { value: 'selector', label: 'Service / Workload (auto-select pod)' },
            ]}
          />
        </div>

        <div class="flex flex-col gap-1">
          <label class="text-xs text-muted" for="pf-target-name">
            {targetKind === 'selector' ? 'Service / workload name' : 'Pod name'}
          </label>
          <input
            id="pf-target-name"
            bind:value={targetName}
            placeholder={targetKind === 'selector' ? 'my-service' : 'my-pod-abc123'}
            class="text-sm bg-surface-hover border border-border rounded px-2 py-1.5 font-mono"
          >
        </div>

        {#if targetKind === 'selector'}
          <div class="flex flex-col gap-1">
            <label class="text-xs text-muted" for="pf-gvr">Resource type (GVR)</label>
            <input
              id="pf-gvr"
              bind:value={targetGVR}
              placeholder="core.v1.services"
              class="text-sm bg-surface-hover border border-border rounded px-2 py-1.5 font-mono"
            >
          </div>
        {/if}

        <div class="flex flex-col gap-1">
          <label class="text-xs text-muted" for="pf-namespace">Namespace</label>
          <input
            id="pf-namespace"
            bind:value={namespace}
            placeholder="default"
            class="text-sm bg-surface-hover border border-border rounded px-2 py-1.5 font-mono"
          >
        </div>

        <div class="grid grid-cols-2 gap-2">
          <div class="flex flex-col gap-1">
            <label class="text-xs text-muted" for="pf-local">Local port</label>
            <input
              id="pf-local"
              bind:value={localPort}
              placeholder="auto"
              class="text-sm bg-surface-hover border border-border rounded px-2 py-1.5 font-mono w-full"
            >
          </div>
          <div class="flex flex-col gap-1">
            <label class="text-xs text-muted" for="pf-remote">Remote port</label>
            <input
              id="pf-remote"
              bind:value={remotePort}
              placeholder="8080"
              class="text-sm bg-surface-hover border border-border rounded px-2 py-1.5 font-mono w-full"
            >
          </div>
        </div>
      {/if}
      {#if !(capabilitiesStore.loaded && !capabilitiesStore.portForwarding)}
        <label class="flex items-center gap-2 text-sm cursor-pointer">
          <input type="checkbox" bind:checked={openInBrowser} class="accent-accent">
          Open in browser after connecting
        </label>
      {/if}
    </div>

    <div class="flex justify-end gap-2 px-4 py-3 border-t border-border">
      <button
        type="button"
        onclick={onclose}
        class="px-3 py-1.5 text-xs rounded border border-border hover:bg-surface-hover transition-colors"
      >
        {capabilitiesStore.loaded && !capabilitiesStore.portForwarding ? 'Close' : 'Cancel'}
      </button>
      {#if !(capabilitiesStore.loaded && !capabilitiesStore.portForwarding)}
        <button
          type="button"
          onclick={submit}
          disabled={submitting || (!isQuickMode && (!(targetName && remotePort)))}
          class="px-3 py-1.5 text-xs rounded bg-accent text-white hover:bg-accent/90 transition-colors disabled:opacity-50"
        >
          {submitting ? 'Starting…' : 'Start'}
        </button>
      {/if}
    </div>
  </div>
</div>
