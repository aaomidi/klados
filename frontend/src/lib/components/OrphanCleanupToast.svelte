<script lang="ts">
  import {onMount} from "svelte";
  import {Events} from "@wailsio/runtime";
  import {clusterStore} from "$lib/stores/cluster.svelte";
  import {notificationStore} from "$lib/stores/notification.svelte";
  import {
    CleanupOrphans,
    TriggerOrphanScan,
  } from "$api/github.com/Vilsol/klados/internal/services/volumebrowserservice.js";
  import type {OrphanPodDTO} from "$api/github.com/Vilsol/klados/internal/services/models.js";
  import {ConnectionStatus} from "$api/github.com/Vilsol/klados/internal/cluster/models.js";
  import {getLogger} from "$lib/logger";

  const log = getLogger("volumebrowser.orphans");

  const subscriptions = new Map<string, () => void>();

  // De-dup window: if the push-event fires shortly after the pull on-mount, we
  // don't want to show two toasts for the same orphan set. Keyed by ctxName.
  const lastShown = new Map<string, {names: Set<string>; at: number}>();
  const DEDUP_WINDOW_MS = 30_000;

  function isSubset(incoming: string[], prior: Set<string>): boolean {
    if (incoming.length === 0) return true;
    for (const n of incoming) {
      if (!prior.has(n)) return false;
    }
    return true;
  }

  function showOrphanToast(ctxName: string, orphans: OrphanPodDTO[]) {
    if (orphans.length === 0) return;

    const names = orphans.map((o) => o.podName);
    const prev = lastShown.get(ctxName);
    const now = Date.now();
    if (prev && now - prev.at < DEDUP_WINDOW_MS && isSubset(names, prev.names)) {
      log.info("suppressing duplicate orphan toast", {ctxName, count: orphans.length});
      return;
    }
    lastShown.set(ctxName, {names: new Set(names), at: now});

    const details = orphans.map((o) => `${o.namespace}/${o.podName} (pvc ${o.pvcName})`).join("\n");
    let toastId = "";
    toastId = notificationStore.push(
      `Found ${orphans.length} leftover volume browser pod${orphans.length === 1 ? "" : "s"} on ${ctxName}`,
      "info",
      {
        details,
        actions: [
          {
            label: "Clean up",
            onClick: () => {
              CleanupOrphans(ctxName).catch((err: unknown) => {
                log.error("CleanupOrphans failed", err);
                notificationStore.error(`Failed to clean up orphans on ${ctxName}`, String(err));
              });
            },
          },
          {
            label: "Dismiss",
            onClick: () => {
              notificationStore.dismiss(toastId);
            },
          },
        ],
      },
    );
  }

  function subscribe(ctxName: string) {
    if (subscriptions.has(ctxName)) {
      return;
    }
    const unsub = Events.On(`volumebrowser:orphans:${ctxName}`, (wailsEvent: {data?: OrphanPodDTO[]}) => {
      const orphans = wailsEvent.data ?? [];
      showOrphanToast(ctxName, orphans);
    });
    subscriptions.set(ctxName, unsub);
  }

  async function pullForConnectedContexts() {
    for (const ctx of clusterStore.contexts) {
      if (ctx.status !== ConnectionStatus.StatusConnected) continue;
      try {
        const orphans = (await TriggerOrphanScan(ctx.name)) as OrphanPodDTO[];
        if (orphans && orphans.length > 0) {
          showOrphanToast(ctx.name, orphans);
        }
      } catch (err) {
        log.error("TriggerOrphanScan failed", {ctxName: ctx.name, err: String(err)});
      }
    }
  }

  onMount(() => {
    // Pull on mount for any already-connected context — covers the auto-reconnect
    // startup path where OnClusterConnected fires before this component subscribes.
    void pullForConnectedContexts();
    return () => {
      for (const unsub of subscriptions.values()) {
        unsub();
      }
      subscriptions.clear();
    };
  });

  $effect(() => {
    for (const ctx of clusterStore.contexts) {
      subscribe(ctx.name);
    }
  });
</script>
