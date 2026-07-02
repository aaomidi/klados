import {mount} from "svelte";
import App from "./App.svelte";
import "@fontsource/jetbrains-mono";
import "./app.css";
import {streamingStore} from "$lib/stores/streaming.svelte.js";

// Patch console so all output is forwarded to the terminal via the streaming server.
// Logs before the streaming server is ready are buffered and flushed on connect.
const queue: string[] = [];

function send(msg: string) {
  const cfg = streamingStore.config;
  if (!cfg) {
    queue.push(msg);
    return;
  }
  while (queue.length > 0) {
    const m = queue.shift() as string;
    fetch(streamingStore.logSinkUrl(), {method: "POST", body: m}).catch(() => {
      /* empty */
    });
  }
  fetch(streamingStore.logSinkUrl(), {method: "POST", body: msg}).catch(() => {
    /* empty */
  });
}

function fmt(prefix: string, args: unknown[]) {
  return prefix + args.map((a) => (typeof a === "object" ? JSON.stringify(a) : String(a))).join(" ");
}

// biome-ignore lint/suspicious/noConsole: intentional console override
const _log = console.log.bind(console);
// biome-ignore lint/suspicious/noConsole: intentional console override
const _warn = console.warn.bind(console);
// biome-ignore lint/suspicious/noConsole: intentional console override
const _error = console.error.bind(console);
// biome-ignore lint/suspicious/noConsole: intentional console override
const _debug = console.debug.bind(console);

console.log = (...a) => {
  _log(...a);
  send(fmt("[LOG] ", a));
};
console.warn = (...a) => {
  _warn(...a);
  send(fmt("[WRN] ", a));
};
console.error = (...a) => {
  _error(...a);
  send(fmt("[ERR] ", a));
};
console.debug = (...a) => {
  _debug(...a);
  send(fmt("[DBG] ", a));
};

// Disable native text-assist globally — macOS WebView otherwise tries to
// autocomplete/autocorrect every input, which breaks comboboxes and k8s name
// fields. No login or address forms in this app, so blanket disable is safe.
function disableAssist(el: Element) {
  if (el instanceof HTMLInputElement || el instanceof HTMLTextAreaElement) {
    if (!el.hasAttribute("autocomplete")) el.setAttribute("autocomplete", "off");
    if (!el.hasAttribute("autocorrect")) el.setAttribute("autocorrect", "off");
    if (!el.hasAttribute("autocapitalize")) el.setAttribute("autocapitalize", "off");
    if (!el.hasAttribute("spellcheck")) el.setAttribute("spellcheck", "false");
  } else if (el instanceof HTMLElement && el.isContentEditable && !el.hasAttribute("spellcheck")) {
    el.setAttribute("spellcheck", "false");
  }
}

function scan(root: ParentNode) {
  if (root instanceof Element) disableAssist(root);
  for (const el of root.querySelectorAll("input, textarea, [contenteditable='true'], [contenteditable='']")) {
    disableAssist(el);
  }
}

scan(document);
new MutationObserver((mutations) => {
  for (const m of mutations) {
    for (const node of m.addedNodes) {
      if (node.nodeType === 1) scan(node as Element);
    }
    if (m.type === "attributes" && m.target instanceof Element) {
      disableAssist(m.target);
    }
  }
}).observe(document.documentElement, {
  childList: true,
  subtree: true,
  attributes: true,
  attributeFilter: ["contenteditable"],
});

const app = mount(App, {target: document.getElementById("app") as HTMLElement});

export default app;
