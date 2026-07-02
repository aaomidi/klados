<script lang="ts">
  import {Dialog} from "bits-ui";
  import {
    DeleteResource,
    ForceDeleteResource,
    ScaleResource,
    RestartResource,
    PauseRollout,
    ResumeRollout,
    DeleteJobCascade,
    DeleteJobOrphan,
    TriggerCronJob,
    SuspendCronJob,
    ResumeCronJob,
    GetResource,
    ExpandPVC,
    RollbackToRevision,
  } from "$api/github.com/Vilsol/klados/internal/services/resourceservice.js";
  import {CordonNode, UncordonNode, StartDrain} from "$api/github.com/Vilsol/klados/internal/services/drainservice.js";
  import {ConfirmDialog, Tooltip} from "@klados/ui";
  import {withBusy} from "$lib/utils/async";
  import {push} from "svelte-spa-router";
  import type {ActionDef} from "$lib/registry/index";
  import {evalExpr} from "$lib/registry/index";
  import {clusterStore} from "$lib/stores/cluster.svelte";
  import {volumeBrowserStore} from "$lib/stores/volumeBrowser.svelte";
  import type {KubernetesResource} from "$lib/types";

  let {
    obj,
    ctxName,
    gvr,
    namespace,
    name,
    actions,
    onrefresh,
  }: {
    obj: KubernetesResource;
    ctxName: string;
    gvr: string;
    namespace: string;
    name: string;
    actions: ActionDef[];
    onrefresh: () => void;
  } = $props();

  let deleteOpen = $state(false);
  let forceDeleteOpen = $state(false);
  let scaleOpen = $state(false);
  let restartOpen = $state(false);
  let rollbackOpen = $state(false);
  let scaleReplicas = $state<number>(1);
  $effect(() => {
    scaleReplicas = obj.spec?.replicas ?? 1;
  });
  let busy = $state(false);

  let expandOpen = $state(false);
  let expandSize = $state("");
  let expandCurrentSize = $state("");
  let expandAllowed = $state<boolean | null>(null);
  let expandError = $state<string | null>(null);
  let expandChecking = $state(false);

  function isDisabled(action: ActionDef): boolean {
    if (!clusterStore.canMutate()) {
      return true;
    }
    if (!action.disabledWhen) {
      return false;
    }
    try {
      return Boolean(evalExpr(action.disabledWhen, obj));
    } catch {
      return false;
    }
  }

  function disabledReason(action: ActionDef): string | undefined {
    if (!clusterStore.canMutate()) {
      return "Read-only mode";
    }
    if (action.disabledWhen && isDisabled(action)) {
      return action.disabledReason;
    }
    return;
  }

  const setBusy = (v: boolean) => {
    busy = v;
  };

  const doDelete = () =>
    withBusy(
      setBusy,
      () => DeleteResource(ctxName, gvr, namespace, name),
      `Deleted ${name}`,
      "Delete failed",
      () => push(`/c/${ctxName}/${gvr}`),
    );

  const doForceDelete = () =>
    withBusy(
      setBusy,
      () => ForceDeleteResource(ctxName, gvr, namespace, name),
      `Force deleted ${name}`,
      "Force delete failed",
      () => push(`/c/${ctxName}/${gvr}`),
    );

  const doScale = () =>
    withBusy(
      setBusy,
      () => ScaleResource(ctxName, gvr, namespace, name, scaleReplicas),
      `Scaled ${name} to ${scaleReplicas}`,
      "Scale failed",
      () => {
        scaleOpen = false;
        onrefresh();
      },
    );

  const doRestart = () =>
    withBusy(setBusy, () => RestartResource(ctxName, gvr, namespace, name), `Restarted ${name}`, "Restart failed", onrefresh);

  const doPause = () => withBusy(setBusy, () => PauseRollout(ctxName, namespace, name), `Paused ${name}`, "Pause failed", onrefresh);

  const doResume = () => withBusy(setBusy, () => ResumeRollout(ctxName, namespace, name), `Resumed ${name}`, "Resume failed", onrefresh);

  const doCordon = () => withBusy(setBusy, () => CordonNode(ctxName, name), `Cordoned ${name}`, "Cordon failed", onrefresh);

  const doUncordon = () => withBusy(setBusy, () => UncordonNode(ctxName, name), `Uncordoned ${name}`, "Uncordon failed", onrefresh);

  const doDrain = () => withBusy(setBusy, () => StartDrain(ctxName, name), `Drain started for ${name}`, "Drain failed");

  const doDeleteJobCascade = () =>
    withBusy(
      setBusy,
      () => DeleteJobCascade(ctxName, namespace, name),
      `Deleted ${name} (cascade)`,
      "Delete failed",
      () => push(`/c/${ctxName}/${gvr}`),
    );

  const doDeleteJobOrphan = () =>
    withBusy(
      setBusy,
      () => DeleteJobOrphan(ctxName, namespace, name),
      `Deleted ${name} (orphan)`,
      "Delete failed",
      () => push(`/c/${ctxName}/${gvr}`),
    );

  const doTriggerCronJob = () => withBusy(setBusy, () => TriggerCronJob(ctxName, namespace, name), `Triggered ${name}`, "Trigger failed");

  const doSuspendCronJob = () =>
    withBusy(setBusy, () => SuspendCronJob(ctxName, namespace, name), `Suspended ${name}`, "Suspend failed", onrefresh);

  const doResumeCronJob = () =>
    withBusy(setBusy, () => ResumeCronJob(ctxName, namespace, name), `Resumed ${name}`, "Resume failed", onrefresh);

  async function openExpand() {
    expandOpen = true;
    expandAllowed = null;
    expandError = null;
    expandChecking = true;
    expandSize = "";
    try {
      const scName: string | undefined = obj.spec?.storageClassName;
      if (!scName) {
        expandAllowed = true;
        expandCurrentSize = obj.status?.capacity?.storage ?? obj.spec?.resources?.requests?.storage ?? "";
        return;
      }
      const sc = await GetResource(ctxName, "storage.k8s.io.v1.storageclasses", "", scName);
      expandAllowed = sc?.spec?.allowVolumeExpansion !== false;
      if (!expandAllowed) {
        expandError = "This StorageClass does not allow volume expansion";
      }
      expandCurrentSize = obj.status?.capacity?.storage ?? obj.spec?.resources?.requests?.storage ?? "";
    } catch {
      expandAllowed = true;
      expandCurrentSize = obj.status?.capacity?.storage ?? obj.spec?.resources?.requests?.storage ?? "";
    } finally {
      expandChecking = false;
    }
  }

  const doExpand = () =>
    withBusy(
      setBusy,
      () => ExpandPVC(ctxName, namespace, name, expandSize),
      `Expanded ${name} to ${expandSize}`,
      "Expand failed",
      () => {
        expandOpen = false;
        onrefresh();
      },
    );

  const destructiveActions = new Set(["delete", "force-delete", "delete-cascade", "delete-orphan"]);

  function getHandler(actionName: string): (() => void) | null {
    switch (actionName) {
      case "scale":
        return () => (scaleOpen = true);
      case "restart":
        return () => (restartOpen = true);
      case "delete":
        return () => (deleteOpen = true);
      case "force-delete":
        return () => (forceDeleteOpen = true);
      case "rollback":
        return () => (rollbackOpen = true);
      case "pause":
        return doPause;
      case "resume":
        return gvr === "batch.v1.cronjobs" ? doResumeCronJob : doResume;
      case "cordon":
        return doCordon;
      case "uncordon":
        return doUncordon;
      case "drain":
        return doDrain;
      case "delete-cascade":
        return doDeleteJobCascade;
      case "delete-orphan":
        return doDeleteJobOrphan;
      case "trigger":
        return doTriggerCronJob;
      case "suspend":
        return doSuspendCronJob;
      case "expand":
        return openExpand;
    }
    return null;
  }
</script>

<div class="flex items-center gap-1.5 px-4 py-2 border-b border-border bg-surface flex-wrap">
  {#if gvr === 'core.v1.persistentvolumeclaims'}
    {@const canBrowse = clusterStore.canMutate()}
    <Tooltip content={canBrowse ? '' : 'Read-only mode'}>
      {#snippet trigger(props)}
        <button
          type="button"
          {...props}
          onclick={(e) => void volumeBrowserStore.spawn(ctxName, namespace, name, { shiftHeld: e.shiftKey })}
          disabled={!canBrowse || busy}
          class="text-xs px-2.5 py-1 rounded border border-border hover:bg-surface-hover transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
        >
          Browse Volume
        </button>
      {/snippet}
    </Tooltip>
  {/if}
  {#each actions.filter(a => !destructiveActions.has(a.name)) as action}
    {@const disabled = isDisabled(action) || busy}
    {@const handler = getHandler(action.name)}
    {@const reason = disabledReason(action)}
    {#if handler}
      {#if reason}
        <Tooltip content={reason}>
          {#snippet trigger(props)}
            <button
              type="button"
              {...props}
              onclick={handler}
              {disabled}
              class="text-xs px-2.5 py-1 rounded border border-border hover:bg-surface-hover transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
            >
              {action.label}
            </button>
          {/snippet}
        </Tooltip>
      {:else}
        <button
          type="button"
          onclick={handler}
          {disabled}
          class="text-xs px-2.5 py-1 rounded border border-border hover:bg-surface-hover transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
        >
          {action.label}
        </button>
      {/if}
    {/if}
  {/each}

  <div class="flex-1"></div>

  {#each actions.filter(a => destructiveActions.has(a.name)) as action}
    {@const disabled = isDisabled(action) || busy}
    {@const handler = getHandler(action.name)}
    {@const reason = disabledReason(action)}
    {#if handler}
      {#if reason}
        <Tooltip content={reason}>
          {#snippet trigger(props)}
            {#if action.name === 'delete'}
              <button
                type="button"
                {...props}
                onclick={handler}
                {disabled}
                class="text-xs px-2.5 py-1 rounded bg-destructive text-destructive-fg hover:opacity-90 transition-opacity disabled:opacity-50"
              >
                {action.label}
              </button>
            {:else}
              <button
                type="button"
                {...props}
                onclick={handler}
                {disabled}
                class="text-xs px-2.5 py-1 rounded border border-destructive text-destructive hover:bg-destructive/10 transition-colors disabled:opacity-40"
              >
                {action.label}
              </button>
            {/if}
          {/snippet}
        </Tooltip>
      {:else if action.name === 'delete'}
        <button
          type="button"
          onclick={handler}
          {disabled}
          class="text-xs px-2.5 py-1 rounded bg-destructive text-destructive-fg hover:opacity-90 transition-opacity disabled:opacity-50"
        >
          {action.label}
        </button>
      {:else}
        <button
          type="button"
          onclick={handler}
          {disabled}
          class="text-xs px-2.5 py-1 rounded border border-destructive text-destructive hover:bg-destructive/10 transition-colors disabled:opacity-40"
        >
          {action.label}
        </button>
      {/if}
    {/if}
  {/each}
</div>

<ConfirmDialog
  bind:open={deleteOpen}
  title="Delete {name}"
  message="This action cannot be undone."
  confirmLabel="Delete"
  onconfirm={doDelete}
/>

<ConfirmDialog
  bind:open={forceDeleteOpen}
  title="Force Delete {name}"
  message="Bypasses graceful termination. The pod will be removed immediately from the API server regardless of node state."
  confirmLabel="Force Delete"
  onconfirm={doForceDelete}
/>

<ConfirmDialog
  bind:open={restartOpen}
  title="Restart {name}"
  message="A rolling restart will be triggered by patching the pod template annotation."
  confirmLabel="Restart"
  onconfirm={doRestart}
/>

<ConfirmDialog
  bind:open={rollbackOpen}
  title="Rollback {name}"
  message="Roll back to the previous revision? This will patch the workload's pod template."
  confirmLabel="Rollback"
  onconfirm={() => withBusy(setBusy,
    () => RollbackToRevision(ctxName, gvr, namespace, name, 0),
    `Rolled back ${name}`, 'Rollback failed', () => { rollbackOpen = false; onrefresh() })}
/>

<!-- Scale dialog -->
<Dialog.Root bind:open={scaleOpen}>
  <Dialog.Portal>
    <Dialog.Overlay class="fixed inset-0 bg-black/50 z-40" />
    <Dialog.Content
      class="fixed left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2 z-50 bg-surface border border-border rounded-lg shadow-xl p-6 w-80"
    >
      <Dialog.Title class="text-base font-semibold mb-4">Scale {name}</Dialog.Title>
      <label for="scale-replicas" class="block text-sm mb-1 text-muted">Replicas</label>
      <input
        id="scale-replicas"
        type="number"
        min="0"
        bind:value={scaleReplicas}
        class="w-full text-sm bg-bg border border-border rounded px-3 py-1.5 focus:outline-none focus:ring-1 focus:ring-accent"
      >
      <div class="flex justify-end gap-2 mt-4">
        <Dialog.Close class="px-3 py-1.5 text-sm rounded border border-border hover:bg-surface-hover transition-colors"
          >Cancel</Dialog.Close
        >
        <button
          type="button"
          onclick={doScale}
          disabled={busy}
          class="px-3 py-1.5 text-sm rounded bg-accent text-accent-fg hover:opacity-90 transition-opacity disabled:opacity-50"
        >
          {busy ? 'Scaling…' : 'Scale'}
        </button>
      </div>
    </Dialog.Content>
  </Dialog.Portal>
</Dialog.Root>

<!-- Expand PVC dialog -->
<Dialog.Root bind:open={expandOpen}>
  <Dialog.Portal>
    <Dialog.Overlay class="fixed inset-0 bg-black/50 z-40" />
    <Dialog.Content
      class="fixed left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2 z-50 bg-surface border border-border rounded-lg shadow-xl p-6 w-80"
    >
      <Dialog.Title class="text-base font-semibold mb-4">Expand PVC</Dialog.Title>
      {#if expandChecking}
        <p class="text-sm text-muted">Checking storage class…</p>
      {:else if expandError}
        <p class="text-sm text-destructive">{expandError}</p>
      {:else}
        {#if expandCurrentSize}
          <p class="text-sm text-muted mb-3">Current size: {expandCurrentSize}</p>
        {/if}
        <label for="expand-size" class="block text-sm mb-1 text-muted">New size</label>
        <input
          id="expand-size"
          type="text"
          placeholder="e.g. 20Gi"
          bind:value={expandSize}
          class="w-full text-sm bg-bg border border-border rounded px-3 py-1.5 focus:outline-none focus:ring-1 focus:ring-accent"
        >
      {/if}
      <div class="flex justify-end gap-2 mt-4">
        <Dialog.Close class="px-3 py-1.5 text-sm rounded border border-border hover:bg-surface-hover transition-colors"
          >Cancel</Dialog.Close
        >
        {#if !(expandChecking || expandError)}
          <button
            type="button"
            onclick={doExpand}
            disabled={busy || !expandSize}
            class="px-3 py-1.5 text-sm rounded bg-accent text-accent-fg hover:opacity-90 transition-opacity disabled:opacity-50"
          >
            {busy ? 'Expanding…' : 'Expand'}
          </button>
        {/if}
      </div>
    </Dialog.Content>
  </Dialog.Portal>
</Dialog.Root>
