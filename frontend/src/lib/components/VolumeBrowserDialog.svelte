<script lang="ts">
  import {Dialog} from "bits-ui";
  import VolumeBrowserSettings from "../../routes/settings/VolumeBrowserSettings.svelte";
  import type {VolumeBrowserConfig} from "$lib/stores/preferences.svelte";
  import type {SpawnOverridesDTO} from "$api/github.com/Vilsol/klados/internal/services/models.js";

  interface Props {
    open: boolean;
    namespace: string;
    pvcName: string;
    initial: VolumeBrowserConfig;
    onsubmit: (overrides: SpawnOverridesDTO) => void;
    oncancel: () => void;
  }

  let {open = $bindable(), namespace, pvcName, initial, onsubmit, oncancel}: Props = $props();

  let value = $state<VolumeBrowserConfig>({} as VolumeBrowserConfig);

  $effect(() => {
    // Re-seed when dialog (re)opens or initial changes
    void open;
    value = {...initial};
  });

  const mountPathInvalid = $derived(
    !!value.mountPath && !value.mountPath.startsWith("/")
  );

  function toOverrides(): SpawnOverridesDTO {
    const o: Partial<SpawnOverridesDTO> = {};
    if (value.image) o.image = value.image;
    if (value.mountPath) o.mountPath = value.mountPath;
    if (value.readOnly != null) o.readOnly = value.readOnly;
    if (value.activeDeadlineSeconds != null) o.activeDeadlineSeconds = value.activeDeadlineSeconds;
    if (value.resources != null) o.resources = value.resources as SpawnOverridesDTO["resources"];
    if (value.nodeSelector && Object.keys(value.nodeSelector).length > 0) {
      o.nodeSelector = value.nodeSelector;
    }
    if (value.tolerations && value.tolerations.length > 0) {
      o.tolerations = value.tolerations;
    }
    return o as SpawnOverridesDTO;
  }

  function submit() {
    if (mountPathInvalid) return;
    onsubmit(toOverrides());
    open = false;
  }

  function cancel() {
    oncancel();
    open = false;
  }
</script>

<Dialog.Root bind:open onOpenChange={(v) => { if (!v) oncancel(); }}>
  <Dialog.Portal>
    <Dialog.Overlay class="fixed inset-0 bg-black/50 z-40" />
    <Dialog.Content
      class="fixed left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2 z-50 bg-bg text-fg border border-border rounded-lg shadow-xl p-6 w-[560px] max-w-[92vw] max-h-[80vh] flex flex-col"
    >
      <Dialog.Title class="text-base font-semibold mb-1">Browse Volume</Dialog.Title>
      <Dialog.Description class="text-xs text-muted mb-4">
        {namespace}/{pvcName}
      </Dialog.Description>

      <div class="flex-1 overflow-auto pr-1">
        <VolumeBrowserSettings bind:value />
      </div>

      <div class="flex justify-end gap-2 mt-4 pt-4 border-t border-border">
        <button
          type="button"
          onclick={cancel}
          class="px-3 py-1.5 text-sm rounded border border-border hover:bg-surface-hover transition-colors"
        >Cancel</button>
        <button
          type="button"
          onclick={submit}
          disabled={mountPathInvalid}
          class="px-3 py-1.5 text-sm rounded bg-accent text-accent-fg hover:opacity-90 transition-opacity disabled:opacity-50"
        >Browse</button>
      </div>
    </Dialog.Content>
  </Dialog.Portal>
</Dialog.Root>
