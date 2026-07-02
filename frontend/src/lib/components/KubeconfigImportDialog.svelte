<script lang="ts">
  import {Dialog} from "bits-ui";
  import {AddKubeconfigPath, ImportKubeconfigContent} from "$api/github.com/Vilsol/klados/internal/services/clusterservice.js";
  import {BrowseKubeconfigFile} from "$api/github.com/Vilsol/klados/internal/services/appservice.js";
  import {capabilitiesStore} from "$lib/stores/capabilities.svelte.js";
  import {notificationStore} from "$lib/stores/notification.svelte.js";
  import {unwrapError} from "$lib/utils/async.js";

  let {
    open = $bindable(false),
    onsuccess,
  }: {
    open: boolean;
    onsuccess: (contexts: unknown[]) => void;
  } = $props();

  let mode = $state<"file" | "paste">("file");
  let filePath = $state("");
  let uploadedName = $state("");
  let uploadedContent = $state("");
  let yamlContent = $state("");
  let error = $state("");
  let loading = $state(false);
  let fileInput = $state<HTMLInputElement>();

  async function onFileChosen(e: Event) {
    error = "";
    const input = e.currentTarget as HTMLInputElement;
    const file = input.files?.[0];
    if (!file) {
      return;
    }
    try {
      uploadedContent = await file.text();
      uploadedName = file.name;
      filePath = "";
    } catch (err: unknown) {
      error = (err as {message?: string})?.message ?? String(err);
    } finally {
      input.value = "";
    }
  }

  // Desktop: native open dialog fills the path (imported by reference, like
  // pre-split). Web: hidden file input imports by content.
  async function browse() {
    if (!capabilitiesStore.nativeDialogs) {
      fileInput?.click();
      return;
    }
    try {
      const path = await BrowseKubeconfigFile();
      if (path) {
        filePath = path;
        uploadedContent = "";
        uploadedName = "";
      }
    } catch (e: unknown) {
      error = (e as {message?: string})?.message ?? String(e);
    }
  }

  async function submit() {
    error = "";
    loading = true;
    try {
      let contexts: unknown[];
      if (mode === "file") {
        if (uploadedContent) {
          contexts = await ImportKubeconfigContent(uploadedContent);
        } else if (filePath.trim()) {
          contexts = await AddKubeconfigPath(filePath.trim());
        } else {
          error = "Choose a file or enter a path on the server";
          return;
        }
      } else {
        if (!yamlContent.trim()) {
          error = "Paste kubeconfig YAML";
          return;
        }
        contexts = await ImportKubeconfigContent(yamlContent.trim());
      }
      open = false;
      filePath = "";
      yamlContent = "";
      uploadedContent = "";
      uploadedName = "";
      const count = (contexts ?? []).length;
      notificationStore.success(`Imported ${count} context${count === 1 ? "" : "s"}`);
      onsuccess(contexts ?? []);
    } catch (e: unknown) {
      error = (e as {message?: string})?.message ?? String(e);
      notificationStore.error("Failed to import kubeconfig", unwrapError(e));
    } finally {
      loading = false;
    }
  }
</script>

<Dialog.Root bind:open>
  <Dialog.Portal>
    <Dialog.Overlay class="fixed inset-0 bg-black/50 z-40" />
    <Dialog.Content
      class="fixed left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2 z-50 bg-surface border border-border rounded-lg shadow-xl p-6 w-[480px] max-w-[90vw]"
    >
      <Dialog.Title class="text-base font-semibold mb-4">Import Kubeconfig</Dialog.Title>

      <!-- Mode tabs -->
      <div class="flex gap-1 mb-4 border-b border-border">
        <button
          type="button"
          onclick={() => { mode = 'file'; error = '' }}
          class="px-3 py-1.5 text-xs font-medium border-b-2 transition-colors
            {mode === 'file' ? 'border-accent text-accent' : 'border-transparent text-muted hover:text-fg'}"
        >
          File
        </button>
        <button
          type="button"
          onclick={() => { mode = 'paste'; error = '' }}
          class="px-3 py-1.5 text-xs font-medium border-b-2 transition-colors
            {mode === 'paste' ? 'border-accent text-accent' : 'border-transparent text-muted hover:text-fg'}"
        >
          Paste YAML
        </button>
      </div>

      {#if mode === 'file'}
        <input
          type="file"
          accept=".yaml,.yml,.conf,.kubeconfig,application/yaml"
          class="hidden"
          bind:this={fileInput}
          onchange={onFileChosen}
        >
        <div class="flex gap-2 mb-2">
          <input
            type="text"
            bind:value={filePath}
            oninput={() => { uploadedContent = ''; uploadedName = '' }}
            placeholder="/path/on/server/kubeconfig.yaml"
            class="flex-1 px-3 py-1.5 text-sm rounded border border-border bg-surface focus:outline-none focus:border-accent"
          >
          <button
            type="button"
            onclick={browse}
            class="px-3 py-1.5 text-sm rounded border border-border hover:bg-surface-hover transition-colors shrink-0"
          >
            {capabilitiesStore.nativeDialogs ? 'Browse…' : 'Upload…'}
          </button>
        </div>
        {#if uploadedName}
          <p class="text-xs text-muted mb-4">Selected: {uploadedName} (imported by content)</p>
        {:else if !capabilitiesStore.nativeDialogs}
          <p class="text-xs text-muted mb-4">Upload a kubeconfig from this machine, or reference a path readable by the server.</p>
        {/if}
      {:else}
        <textarea
          bind:value={yamlContent}
          placeholder="Paste kubeconfig YAML here..."
          rows={10}
          class="w-full px-3 py-2 text-xs font-mono rounded border border-border bg-surface focus:outline-none focus:border-accent resize-none mb-4"
        ></textarea>
      {/if}

      {#if error}
        <p class="text-xs text-destructive mb-3">{error}</p>
      {/if}

      <div class="flex justify-end gap-2">
        <Dialog.Close class="px-3 py-1.5 text-sm rounded border border-border hover:bg-surface-hover transition-colors">
          Cancel
        </Dialog.Close>
        <button
          type="button"
          onclick={submit}
          disabled={loading}
          class="px-3 py-1.5 text-sm rounded bg-accent text-accent-fg hover:opacity-90 transition-opacity disabled:opacity-50"
        >
          {loading ? 'Importing...' : 'Import'}
        </button>
      </div>
    </Dialog.Content>
  </Dialog.Portal>
</Dialog.Root>
