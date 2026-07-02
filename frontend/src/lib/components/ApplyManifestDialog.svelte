<script lang="ts">
  import {Dialog} from "bits-ui";
  import {cmYamlExtensions} from "@klados/ui";
  import {onDestroy} from "svelte";
  import {EditorView} from "@codemirror/view";
  import {EditorState} from "@codemirror/state";
  import {ApplyManifest} from "$api/github.com/Vilsol/klados/internal/services/resourceservice.js";
  import {BrowseManifestFile} from "$api/github.com/Vilsol/klados/internal/services/appservice.js";
  import {capabilitiesStore} from "$lib/stores/capabilities.svelte.js";
  import {notificationStore} from "$lib/stores/notification.svelte";
  import {shortcutActions} from "$lib/stores/shortcutActions.svelte";

  interface ApplyResult {
    gvr: string;
    namespace: string;
    name: string;
    action: string;
    error: string;
  }

  let {
    open = $bindable(false),
    ctxName,
  }: {
    open: boolean;
    ctxName: string;
  } = $props();

  let container: HTMLDivElement | undefined = $state();
  let view: EditorView | undefined;
  let applying = $state(false);
  let results = $state<ApplyResult[]>([]);
  let hasApplied = $state(false);
  let editorContent = $state("");

  function initEditor(doc = "") {
    view?.destroy();
    view = new EditorView({
      state: EditorState.create({
        doc,
        extensions: [
          ...cmYamlExtensions(),
          EditorView.updateListener.of((update) => {
            if (update.docChanged) {
              editorContent = update.state.doc.toString();
            }
          }),
        ],
      }),
      parent: container as HTMLDivElement,
    });
  }

  onDestroy(() => view?.destroy());

  $effect(() => {
    if (open && container && !view) {
      initEditor();
    }
    if (!open) {
      view?.destroy();
      view = undefined;
      results = [];
      hasApplied = false;
      editorContent = "";
    }
  });

  function loadContent(text: string) {
    editorContent = text;
    if (view) {
      view.dispatch({changes: {from: 0, to: view.state.doc.length, insert: text}});
    } else if (container) {
      initEditor(text);
    }
  }

  let manifestFileInput = $state<HTMLInputElement>();

  // Desktop: native open dialog (returns the file's content, like
  // pre-split). Web: hidden file input.
  async function openFile() {
    if (!capabilitiesStore.nativeDialogs) {
      manifestFileInput?.click();
      return;
    }
    try {
      const content = await BrowseManifestFile();
      if (content) {
        loadContent(content);
      }
    } catch (e: unknown) {
      notificationStore.push((e as {message?: string})?.message ?? "Could not open file", "error");
    }
  }

  async function onManifestFileChosen(e: Event) {
    const input = e.currentTarget as HTMLInputElement;
    const file = input.files?.[0];
    if (!file) {
      return;
    }
    try {
      const content = await file.text();
      if (content) {
        loadContent(content);
      }
    } catch (err: unknown) {
      notificationStore.push((err as {message?: string})?.message ?? "Could not open file", "error");
    } finally {
      input.value = "";
    }
  }

  async function pasteFromClipboard() {
    try {
      const text = await navigator.clipboard.readText();
      if (text.trim()) {
        loadContent(text);
      }
    } catch {
      notificationStore.push("Could not read clipboard", "error");
    }
  }

  const docCount = $derived(
    editorContent.trim() ? editorContent.split("---").filter((s) => s.trim() && !s.trim().startsWith("#")).length : 0,
  );

  const editorEmpty = $derived(!editorContent.trim());

  async function applyManifest() {
    if (!view) {
      return;
    }
    applying = true;
    hasApplied = false;
    try {
      const yaml = view.state.doc.toString();
      const res = await ApplyManifest(ctxName, yaml);
      results = (res ?? []) as ApplyResult[];
      hasApplied = true;
    } catch (e: unknown) {
      notificationStore.push((e as {message?: string})?.message ?? "Apply failed", "error");
    } finally {
      applying = false;
    }
  }

  $effect(() => {
    shortcutActions.confirmDialog;
    if (shortcutActions.confirmDialog > 0 && open && !applying && !editorEmpty) {
      applyManifest();
    }
  });

  function actionClass(action: string, error: string): string {
    if (error) {
      return "text-destructive";
    }
    if (action === "created") {
      return "text-green-400";
    }
    if (action === "configured") {
      return "text-blue-400";
    }
    return "text-muted";
  }
</script>

<Dialog.Root bind:open>
  <Dialog.Portal>
    <Dialog.Overlay class="fixed inset-0 bg-black/50 z-40" />
    <Dialog.Content
      class="fixed left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2 z-50 bg-surface border border-border rounded-lg shadow-xl flex flex-col"
      style="width: min(800px, 92vw); height: min(700px, 90vh);"
    >
      <div class="flex items-center gap-2 px-4 py-3 border-b border-border shrink-0">
        <Dialog.Title class="text-sm font-semibold flex-1">Apply Manifest</Dialog.Title>
        <input
          type="file"
          accept=".yaml,.yml,application/yaml"
          class="hidden"
          bind:this={manifestFileInput}
          onchange={onManifestFileChosen}
        >
        <button
          type="button"
          onclick={openFile}
          class="text-xs px-2.5 py-1 rounded border border-border hover:bg-surface-hover transition-colors"
        >
          Open File…
        </button>
        <button
          type="button"
          onclick={pasteFromClipboard}
          class="text-xs px-2.5 py-1 rounded border border-border hover:bg-surface-hover transition-colors"
        >
          Paste from Clipboard
        </button>
        <Dialog.Close class="text-xs px-2.5 py-1 rounded border border-border hover:bg-surface-hover transition-colors"
          >Cancel</Dialog.Close
        >
        <button
          type="button"
          onclick={applyManifest}
          disabled={applying || editorEmpty}
          class="text-xs px-2.5 py-1 rounded bg-accent text-accent-fg hover:opacity-90 transition-opacity disabled:opacity-50"
        >
          {#if applying}
            Applying…
          {:else if docCount > 0}
            Apply ({docCount}
            resource{docCount === 1 ? '' : 's'})
          {:else}
            Apply
          {/if}
        </button>
      </div>

      <div bind:this={container} class="flex-1 overflow-hidden min-h-0"></div>

      {#if hasApplied}
        <div class="border-t border-border shrink-0 max-h-48 overflow-y-auto px-4 py-2 flex flex-col gap-1">
          {#each results as r}
            <div class="flex items-start gap-2 text-xs font-mono">
              <span class="text-muted shrink-0">{r.gvr}/{r.namespace || '—'}/{r.name || '—'}</span>
              {#if r.error}
                <span class="text-destructive break-all">{r.error}</span>
              {:else}
                <span class={actionClass(r.action, r.error)}>{r.action}</span>
              {/if}
            </div>
          {/each}
        </div>
      {/if}
    </Dialog.Content>
  </Dialog.Portal>
</Dialog.Root>
