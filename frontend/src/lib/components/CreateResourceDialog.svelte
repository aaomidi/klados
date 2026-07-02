<script lang="ts">
  import {Dialog} from "bits-ui";
  import {Combobox, cmYamlExtensions} from "@klados/ui";
  import {onDestroy} from "svelte";
  import {EditorView, hoverTooltip} from "@codemirror/view";
  import {EditorState} from "@codemirror/state";
  import {linter} from "@codemirror/lint";
  import {yamlSchemaLinter, yamlSchemaHover} from "codemirror-json-schema/yaml";
  import {stateExtensions, handleRefresh} from "codemirror-json-schema";
  import {yamlSchemaCompletion} from "codemirror-yaml-completion";
  import {yaml as yamlLang} from "@codemirror/lang-yaml";
  import {parse} from "yaml";
  import {
    GetAllTemplateGVRs,
    GetTemplates,
    CreateResource,
  } from "$api/github.com/Vilsol/klados/internal/services/resourceservice.js";
  import {GetSchema} from "$api/github.com/Vilsol/klados/internal/services/schemaservice.js";
  import {notificationStore} from "$lib/stores/notification.svelte";
  import {shortcutActions} from "$lib/stores/shortcutActions.svelte";
  import {getLogger} from "$lib/logger";

  const log = getLogger("resources");

  const NAME_LINE_RE = /^( +)name:/m;
  const NAME_NS_INJECT_RE = /^( +name:[^\n]*\n)/m;

  interface TemplateItem {
    gvr: string;
    name: string;
    description: string;
    content: string;
    source: string;
  }

  let {
    open = $bindable(false),
    ctxName,
    gvr: initialGvr = "",
    defaultNamespace = "default",
    onsuccess,
  }: {
    open: boolean;
    ctxName: string;
    gvr?: string;
    defaultNamespace?: string;
    onsuccess?: () => void;
  } = $props();

  let container: HTMLDivElement | undefined = $state();
  let view: EditorView | undefined;
  let saving = $state(false);
  // svelte-ignore state_referenced_locally
  let selectedGvr = $state(initialGvr);
  let allGvrs = $state<string[]>([]);
  let templates = $state<TemplateItem[]>([]);
  let selectedTemplateName = $state("");
  let prevSelectedTemplateName = $state("");
  let editorDirty = $state(false);

  let currentSchema: Record<string, unknown> | null = null;

  function initEditor(doc: string, schema?: Record<string, unknown> | null) {
    const cursorPos = view ? Math.min(view.state.selection.main.head, doc.length) : 0;
    view?.destroy();
    view = new EditorView({
      state: EditorState.create({
        doc,
        selection: {anchor: cursorPos},
        extensions: [
          ...cmYamlExtensions({
            lang: schema ? [
              yamlLang(),
              linter(yamlSchemaLinter(), {needsRefresh: handleRefresh}),
              hoverTooltip(yamlSchemaHover()),
              stateExtensions(schema),
              yamlSchemaCompletion(schema),
            ] : undefined,
          }),
          EditorView.updateListener.of((update) => {
            if (update.docChanged) {
              editorDirty = true;
            }
          }),
        ],
      }),
      parent: container as HTMLDivElement,
    });
    editorDirty = false;
  }

  onDestroy(() => view?.destroy());

  $effect(() => {
    if (open && ctxName) {
      GetAllTemplateGVRs(ctxName)
        .then((gvrs: string[]) => {
          allGvrs = gvrs;
        })
        .catch((e) => log.warn("Failed to fetch template GVRs", {error: String(e)}));
    }
  });

  $effect(() => {
    if (open) {
      selectedGvr = initialGvr;
    }
  });

  $effect(() => {
    if (selectedGvr && ctxName) {
      const gvr = selectedGvr;
      GetTemplates(ctxName, gvr)
        .then((t: TemplateItem[]) => {
          if (selectedGvr !== gvr) {
            return;
          }
          templates = t;
          if (t.length > 0) {
            loadTemplate(t[0]);
          }
        })
        .catch((e) => log.warn("Failed to fetch templates", {error: String(e)}));
    } else {
      templates = [];
      selectedTemplateName = "";
    }
  });

  $effect(() => {
    if (open && container && !view) {
      initEditor("", currentSchema);
      // Trigger schema fetch for the initial GVR
      if (selectedGvr && ctxName) {
        fetchAndApplySchema(selectedGvr);
      }
    }
    if (!open && view) {
      view.destroy();
      view = undefined;
      lastSchemaGvr = "";
      currentSchema = null;
    }
  });

  // Fetch schema when GVR changes; rebuild editor to apply it.
  let lastSchemaGvr = "";

  function fetchAndApplySchema(gvr: string) {
    if (!gvr || !ctxName || !view) {
      return;
    }
    if (gvr === lastSchemaGvr) {
      return;
    }
    lastSchemaGvr = gvr;
    GetSchema(ctxName, gvr, "")
      .then((schema) => {
        if (view && schema && Object.keys(schema).length > 0) {
          currentSchema = schema;
          const doc = view.state.doc.toString();
          initEditor(doc, schema);
        }
      })
      .catch((e) => log.warn("Failed to fetch schema for autocomplete", {error: String(e)}));
  }

  $effect(() => {
    const gvr = selectedGvr;
    if (gvr && ctxName && view) {
      fetchAndApplySchema(gvr);
    }
  });

  function injectNamespace(content: string, ns: string): string {
    if (!ns || content.includes("namespace:")) {
      return content;
    }
    const nameMatch = content.match(NAME_LINE_RE);
    if (!nameMatch) {
      return content;
    }
    const indent = nameMatch[1];
    return content.replace(NAME_NS_INJECT_RE, `$1${indent}namespace: ${ns}\n`);
  }

  function loadTemplateContent(tmpl: TemplateItem) {
    const content = injectNamespace(tmpl.content, defaultNamespace);
    if (view) {
      view.dispatch({changes: {from: 0, to: view.state.doc.length, insert: content}});
      editorDirty = false;
    } else if (container) {
      initEditor(content);
    }
  }

  // Used by the GVR-change $effect to auto-load the first template (also sets selectedTemplateName).
  function loadTemplate(tmpl: TemplateItem) {
    selectedTemplateName = tmpl.name;
    prevSelectedTemplateName = tmpl.name;
    loadTemplateContent(tmpl);
  }

  function onTemplateValueChange(name: string) {
    const tmpl = templates.find((t) => t.name === name);
    if (!tmpl) {
      return;
    }
    // biome-ignore lint/suspicious/noAlert: intentional browser confirm for destructive action
    if (editorDirty && !confirm("Replace current YAML with selected template?")) {
      selectedTemplateName = prevSelectedTemplateName;
      return;
    }
    prevSelectedTemplateName = name;
    loadTemplateContent(tmpl);
  }

  async function importFromClipboard() {
    try {
      const text = await navigator.clipboard.readText();
      if (text.trim()) {
        view?.dispatch({changes: {from: 0, to: view?.state.doc.length, insert: text}});
        editorDirty = true;
      }
    } catch {
      notificationStore.push("Could not read clipboard", "error");
    }
  }

  async function apply() {
    if (!view) {
      return;
    }
    saving = true;
    try {
      const yamlText = view.state.doc.toString();
      const parsed = parse(yamlText) as import("$lib/types").KubernetesResource;
      if (!parsed) {
        notificationStore.push("Invalid YAML", "error");
        return;
      }
      const ns = parsed.metadata?.namespace || defaultNamespace;
      const gvrToUse = selectedGvr || "";
      await CreateResource(ctxName, gvrToUse, ns, parsed);
      notificationStore.push(`Created ${parsed.metadata?.name ?? "resource"}`, "success");
      open = false;
      onsuccess?.();
    } catch (e: unknown) {
      notificationStore.push((e as {message?: string})?.message ?? "Create failed", "error");
    } finally {
      saving = false;
    }
  }

  $effect(() => {
    shortcutActions.confirmDialog;
    if (shortcutActions.confirmDialog > 0 && open && !saving) {
      apply();
    }
  });
</script>

<Dialog.Root bind:open>
  <Dialog.Portal>
    <Dialog.Overlay class="fixed inset-0 bg-black/50 z-40" />
    <Dialog.Content
      class="fixed left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2 z-50 bg-surface border border-border rounded-lg shadow-xl flex flex-col"
      style="width: min(800px, 92vw); height: min(640px, 85vh);"
    >
      <div class="flex items-center gap-2 px-4 py-3 border-b border-border shrink-0">
        <Dialog.Title class="text-sm font-semibold flex-1">Create Resource</Dialog.Title>
        <button
          type="button"
          onclick={importFromClipboard}
          class="text-xs px-2.5 py-1 rounded border border-border hover:bg-surface-hover transition-colors"
        >
          Import from Clipboard
        </button>
        <Dialog.Close class="text-xs px-2.5 py-1 rounded border border-border hover:bg-surface-hover transition-colors"
          >Cancel</Dialog.Close
        >
        <button
          type="button"
          onclick={apply}
          disabled={saving}
          class="text-xs px-2.5 py-1 rounded bg-accent text-accent-fg hover:opacity-90 transition-opacity disabled:opacity-50"
        >
          {saving ? 'Creating…' : 'Create'}
        </button>
      </div>

      <div class="flex items-center gap-3 px-4 py-2 border-b border-border shrink-0 flex-wrap">
        <div class="flex items-center gap-2 min-w-0 flex-1">
          <span class="text-xs text-muted whitespace-nowrap">Resource Type</span>
          <div class="flex-1 min-w-0">
            <Combobox
              bind:value={selectedGvr}
              options={allGvrs.map((g) => ({ value: g, label: g }))}
              placeholder="Select resource type…"
              searchPlaceholder="Search GVRs…"
              size="xs"
            />
          </div>
        </div>
        {#if templates.length > 0}
          <div class="flex items-center gap-2 min-w-0 flex-1">
            <span class="text-xs text-muted whitespace-nowrap">Template</span>
            <div class="flex-1 min-w-0">
              <Combobox
                bind:value={selectedTemplateName}
                options={templates.map((t) => ({ value: t.name, label: t.name }))}
                placeholder="Select template…"
                searchPlaceholder="Search templates…"
                size="xs"
                onValueChange={onTemplateValueChange}
              />
            </div>
          </div>
        {/if}
      </div>

      <div bind:this={container} class="flex-1 overflow-hidden"></div>
    </Dialog.Content>
  </Dialog.Portal>
</Dialog.Root>
