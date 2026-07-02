<script lang="ts">
  import {PanelLeftClose, PanelLeft, ChevronRight, X, Circle, Puzzle, LayoutList, Settings} from "lucide-svelte";
  import {sessionStore} from "$lib/stores/session.svelte";
  import {clusterStore} from "$lib/stores/cluster.svelte";
  import {Events} from "@wailsio/runtime";
  import {push, router} from "svelte-spa-router";
  import {onDestroy} from "svelte";
  import {ListAPIResources} from "$api/github.com/Vilsol/klados/internal/services/resourceservice.js";
  import {ListForwards, StopForward} from "$api/github.com/Vilsol/klados/internal/services/portforwardservice.js";
  import {GetPluginSidebarEntries} from "$api/github.com/Vilsol/klados/internal/services/pluginservice.js";
  import type {APIResource} from "$api/github.com/Vilsol/klados/internal/cluster/index.js";
  import {notificationStore} from "$lib/stores/notification.svelte.js";
  import {unwrapError} from "$lib/utils/async.js";
  import {descriptorRegistry} from "$lib/registry/index";
  import {registryLoaded} from "$lib/registry/loaded.svelte";
  import {buildCRDTree} from "$lib/utils/crdTree";
  import CRDTreeNode from "./CRDTreeNode.svelte";
  import {getLogger} from "$lib/logger";
  import SidebarResizeHandle from "./SidebarResizeHandle.svelte";

  const log = getLogger("sidebar");

  const gvrGroups: Record<string, string[]> = {
    Workloads: [
      "core.v1.pods",
      "apps.v1.deployments",
      "apps.v1.statefulsets",
      "apps.v1.daemonsets",
      "apps.v1.replicasets",
      "batch.v1.jobs",
      "batch.v1.cronjobs",
      "autoscaling.v2.horizontalpodautoscalers",
      "policy.v1.poddisruptionbudgets",
    ],
    Networking: [
      "core.v1.services",
      "networking.k8s.io.v1.ingresses",
      "networking.k8s.io.v1.networkpolicies",
      "networking.k8s.io.v1.ingressclasses",
      "discovery.k8s.io.v1.endpointslices",
    ],
    Config: ["core.v1.configmaps", "core.v1.secrets", "core.v1.resourcequotas", "core.v1.limitranges", "coordination.k8s.io.v1.leases"],
    Storage: [
      "core.v1.persistentvolumeclaims",
      "core.v1.persistentvolumes",
      "storage.k8s.io.v1.storageclasses",
      "storage.k8s.io.v1.csidrivers",
    ],
    Cluster: [
      "core.v1.namespaces",
      "core.v1.nodes",
      "apiextensions.k8s.io.v1.customresourcedefinitions",
      "admissionregistration.k8s.io.v1.mutatingwebhookconfigurations",
      "admissionregistration.k8s.io.v1.validatingwebhookconfigurations",
      "scheduling.k8s.io.v1.priorityclasses",
      "node.k8s.io.v1.runtimeclasses",
    ],
    RBAC: [
      "core.v1.serviceaccounts",
      "rbac.authorization.k8s.io.v1.roles",
      "rbac.authorization.k8s.io.v1.clusterroles",
      "rbac.authorization.k8s.io.v1.rolebindings",
      "rbac.authorization.k8s.io.v1.clusterrolebindings",
    ],
  };

  const kindByGvr: Record<string, string> = {
    "core.v1.pods": "Pods",
    "apps.v1.deployments": "Deployments",
    "apps.v1.statefulsets": "StatefulSets",
    "apps.v1.daemonsets": "DaemonSets",
    "apps.v1.replicasets": "ReplicaSets",
    "batch.v1.jobs": "Jobs",
    "batch.v1.cronjobs": "CronJobs",
    "core.v1.services": "Services",
    "networking.k8s.io.v1.ingresses": "Ingresses",
    "core.v1.configmaps": "ConfigMaps",
    "core.v1.secrets": "Secrets",
    "core.v1.persistentvolumeclaims": "PersistentVolumeClaims",
    "core.v1.persistentvolumes": "PersistentVolumes",
    "storage.k8s.io.v1.storageclasses": "StorageClasses",
    "storage.k8s.io.v1.csidrivers": "CSI Drivers",
    "core.v1.namespaces": "Namespaces",
    "core.v1.nodes": "Nodes",
    "apiextensions.k8s.io.v1.customresourcedefinitions": "CRDs",
    "core.v1.serviceaccounts": "ServiceAccounts",
    "rbac.authorization.k8s.io.v1.roles": "Roles",
    "rbac.authorization.k8s.io.v1.clusterroles": "ClusterRoles",
    "rbac.authorization.k8s.io.v1.rolebindings": "RoleBindings",
    "rbac.authorization.k8s.io.v1.clusterrolebindings": "ClusterRoleBindings",
    "autoscaling.v2.horizontalpodautoscalers": "HPAs",
    "policy.v1.poddisruptionbudgets": "PodDisruptionBudgets",
    "networking.k8s.io.v1.networkpolicies": "NetworkPolicies",
    "networking.k8s.io.v1.ingressclasses": "IngressClasses",
    "discovery.k8s.io.v1.endpointslices": "EndpointSlices",
    "core.v1.resourcequotas": "ResourceQuotas",
    "core.v1.limitranges": "LimitRanges",
    "coordination.k8s.io.v1.leases": "Leases",
    "admissionregistration.k8s.io.v1.mutatingwebhookconfigurations": "MutatingWebhookConfigs",
    "admissionregistration.k8s.io.v1.validatingwebhookconfigurations": "ValidatingWebhookConfigs",
    "scheduling.k8s.io.v1.priorityclasses": "PriorityClasses",
    "node.k8s.io.v1.runtimeclasses": "RuntimeClasses",
  };

  let discoveredGVRs = $state<Set<string>>(new Set());
  let expandedGroups = $state<Record<string, boolean>>({Workloads: true});
  let customResources = $state<APIResource[]>([]);

  const crdTree = $derived(
    (() => {
      const kindMap = new Map(customResources.map((r) => [r.gvr, r.kind]));
      return buildCRDTree(
        customResources.map((r) => r.gvr),
        (gvr) => kindMap.get(gvr) || gvr.split(".").at(-1) || gvr,
      );
    })(),
  );

  let expandedNodes = $state(new Set<string>());
  $effect(() => {
    void clusterStore.activeContext; // track
    expandedNodes = new Set();
  });

  function toggleExpand(fullSuffix: string) {
    const next = new Set(expandedNodes);
    if (next.has(fullSuffix)) {
      next.delete(fullSuffix);
    } else {
      next.add(fullSuffix);
    }
    expandedNodes = next;
  }

  interface ForwardSpec {
    id: string;
    contextName: string;
    targetName: string;
    localPort: number;
    remotePort: number;
    status: string;
    podName: string;
    error: string;
  }

  interface PluginSidebarEntry {
    category: string;
    label: string;
    gvr: string;
    icon: string;
    plugin: string;
  }

  let forwards = $state<ForwardSpec[]>([]);
  let pluginEntries = $state<PluginSidebarEntry[]>([]);

  async function loadPluginEntries() {
    try {
      const result = await GetPluginSidebarEntries();
      pluginEntries = (result ?? []) as PluginSidebarEntry[];
    } catch {
      // ignore
    }
  }

  async function loadForwards() {
    if (!ctx) {
      return;
    }
    try {
      const result = await ListForwards(ctx);
      forwards = (result ?? []) as ForwardSpec[];
    } catch {
      // ignore
    }
  }

  const ctx = $derived(clusterStore.activeContext);

  function handleDiscovery(resources: APIResource[]) {
    const gvrs = new Set(resources.map((r) => r.gvr));
    discoveredGVRs = gvrs;
    descriptorRegistry.setAvailableGVRs(Array.from(gvrs));
    clusterStore.setDiscoveryResources(resources);

    const knownGVRs = new Set(Object.values(gvrGroups).flat());
    // Show resources not in any builtin group and not purely internal/meta resources
    const internalPrefixes = [
      "core.v1.",
      "rbac.authorization.k8s.io.",
      "authorization.k8s.io.",
      "authentication.k8s.io.",
      "events.k8s.io.",
    ];
    customResources = resources.filter((r) => !(knownGVRs.has(r.gvr) || internalPrefixes.some((p) => r.gvr.startsWith(p))));
  }

  let unsub: (() => void) | null = null;
  let unsubPF: (() => void) | null = null;
  let unsubPlugins: (() => void) | null = null;

  $effect(() => {
    if (unsub) {
      unsub();
      unsub = null;
    }
    if (unsubPF) {
      unsubPF();
      unsubPF = null;
    }
    if (unsubPlugins) {
      unsubPlugins();
      unsubPlugins = null;
    }
    if (ctx) {
      unsub = Events.On(`discovery:${ctx}:resources`, (wailsEvent: {data?: APIResource[]}) => {
        handleDiscovery((wailsEvent.data ?? []) as APIResource[]);
      });
      ListAPIResources(ctx)
        .then((r) => {
          if (r?.length > 0) {
            handleDiscovery(r as APIResource[]);
          }
        })
        .catch((e) => log.warn("ListAPIResources failed", {error: String(e)}));

      loadForwards();
      unsubPF = Events.On(`portforward:${ctx}:updated`, () => {
        loadForwards();
      });
    }

    loadPluginEntries();
    unsubPlugins = Events.On("plugins:loaded", () => {
      loadPluginEntries();
    });
  });

  onDestroy(() => {
    unsub?.();
    unsubPF?.();
    unsubPlugins?.();
  });

  async function stopForward(id: string) {
    // Optimistically remove from the sidebar so the click is visually
    // immediate. A trailing `portforward:{ctx}:updated` event from the
    // backend's StatusStopped emission would otherwise race with our
    // post-stop refresh and momentarily re-add the entry.
    forwards = forwards.filter((f) => f.id !== id);
    try {
      await StopForward(id);
      await loadForwards();
      notificationStore.success("Port forward stopped");
    } catch (e: unknown) {
      await loadForwards();
      notificationStore.error("Failed to stop port forward", unwrapError(e));
    }
  }

  const activePath = $derived(router.location);
  function isActive(path: string) {
    return activePath === path;
  }
  function isGVRActive(gvr: string) {
    return ctx ? activePath === `/c/${ctx}/${gvr}` : false;
  }

  function navigate(gvr: string) {
    if (!ctx) {
      return;
    }
    push(`/c/${ctx}/${gvr}`);
  }

  function toggleGroup(name: string) {
    expandedGroups[name] = !expandedGroups[name];
  }
</script>

<aside
  class="relative border-r border-border bg-surface shrink-0 overflow-hidden transition-all duration-200"
  class:w-0={sessionStore.sidebarCollapsed}
  style={sessionStore.sidebarCollapsed ? "" : `width: ${sessionStore.sidebarWidth}px`}
>
  <div class="h-full flex flex-col">
    <div class="flex items-center justify-between px-3 py-2 border-b border-border">
      <span class="text-xs font-semibold uppercase tracking-wider text-muted">Resources</span>
      <button
        type="button"
        onclick={() => sessionStore.toggleSidebar()}
        class="p-1 rounded hover:bg-surface-hover transition-colors"
        aria-label="Collapse sidebar"
      >
        <PanelLeftClose size={14} />
      </button>
    </div>

    <nav class="flex-1 overflow-y-auto py-1">
      {#if ctx}
        <button
          type="button"
          onclick={() => push(`/c/${ctx}`)}
          class="w-full text-left px-3 py-1.5 text-sm flex items-center gap-2 rounded-none hover:bg-surface-hover transition-colors border-b border-border mb-1 {isActive(`/c/${ctx}`) ? 'bg-surface-hover text-accent font-medium' : 'text-fg'}"
        >
          Overview
        </button>
      {/if}
      {#each Object.entries(gvrGroups) as [ groupName, gvrs ]}
        <div>
          <button
            type="button"
            onclick={() => toggleGroup(groupName)}
            class="w-full flex items-center gap-1 px-3 py-1.5 text-xs font-semibold uppercase tracking-wider text-muted hover:bg-surface-hover transition-colors"
          >
            <ChevronRight size={12} class="transition-transform {expandedGroups[groupName] ? 'rotate-90' : ''}" />
            {groupName}
          </button>
          {#if expandedGroups[groupName]}
            <div class="ml-4">
              {#each gvrs as gvr}
                {@const unavailable = ctx && !descriptorRegistry.isGVRAvailable(gvr)}
                <button
                  type="button"
                  onclick={() => { if (!unavailable) { navigate(gvr); } }}
                  disabled={Boolean(unavailable)}
                  title={unavailable ? "API group not available on this cluster" : undefined}
                  class="w-full text-left px-3 py-1 text-sm transition-colors rounded-sm {unavailable ? 'opacity-40 cursor-not-allowed text-muted' : isGVRActive(gvr) ? 'bg-surface-hover text-accent font-medium' : 'hover:bg-surface-hover'}"
                >
                  {kindByGvr[gvr] ?? gvr.split('.').at(-1)}
                </button>
              {/each}
            </div>
          {/if}
        </div>
      {/each}

      {#if crdTree.length > 0}
        <div>
          <button
            type="button"
            onclick={() => toggleGroup('Custom Resources')}
            class="w-full flex items-center gap-1 px-3 py-1.5 text-xs font-semibold uppercase tracking-wider text-muted hover:bg-surface-hover transition-colors"
          >
            <ChevronRight size={12} class="transition-transform {expandedGroups['Custom Resources'] ? 'rotate-90' : ''}" />
            Custom Resources
          </button>
          {#if expandedGroups['Custom Resources']}
            <div class="ml-4">
              {#each crdTree as node}
                <CRDTreeNode {node} expanded={expandedNodes} onToggle={toggleExpand} ctxName={ctx ?? ''} {activePath} />
              {/each}
            </div>
          {/if}
        </div>
      {/if}
      {#if pluginEntries.length > 0}
        {@const pluginCategories = [...new Set(pluginEntries.map((e) => e.category))]}
        {#each pluginCategories as category}
          <div>
            <button
              type="button"
              onclick={() => toggleGroup(`plugin:${category}`)}
              class="w-full flex items-center gap-1 px-3 py-1.5 text-xs font-semibold uppercase tracking-wider text-muted hover:bg-surface-hover transition-colors"
            >
              <ChevronRight size={12} class="transition-transform {expandedGroups[`plugin:${category}`] ? 'rotate-90' : ''}" />
              {category}
            </button>
            {#if expandedGroups[`plugin:${category}`]}
              <div class="ml-4">
                {#each pluginEntries.filter((e) => e.category === category) as entry}
                  <button
                    type="button"
                    onclick={() => navigate(entry.gvr)}
                    class="w-full text-left px-3 py-1 text-sm transition-colors rounded-sm {isGVRActive(entry.gvr) ? 'bg-surface-hover text-accent font-medium' : 'hover:bg-surface-hover'}"
                  >
                    {entry.label}
                  </button>
                {/each}
              </div>
            {/if}
          </div>
        {/each}
      {/if}

      {#if ctx}
        <div class="border-t border-border mt-1 pt-1">
          <button
            type="button"
            onclick={() => push(`/c/${ctx}/events`)}
            class="w-full flex items-center gap-2 px-3 py-1.5 text-xs font-medium hover:bg-surface-hover transition-colors {isActive(`/c/${ctx}/events`) ? 'bg-surface-hover text-accent' : 'text-muted'}"
          >
            Event Stream
          </button>
        </div>
      {/if}
    </nav>

    <!-- Plugins link -->
    <div class="border-t border-border">
      <button
        type="button"
        onclick={() => push('/plugins')}
        class="w-full flex items-center gap-2 px-3 py-1.5 text-xs font-medium hover:bg-surface-hover transition-colors {isActive('/plugins') ? 'bg-surface-hover text-accent' : 'text-muted'}"
      >
        <Puzzle size={12} />
        Plugins
      </button>
      <button
        type="button"
        onclick={() => push('/settings')}
        class="w-full flex items-center gap-2 px-3 py-1.5 text-xs font-medium hover:bg-surface-hover transition-colors {activePath.startsWith('/settings') ? 'bg-surface-hover text-accent' : 'text-muted'}"
      >
        <Settings size={12} />
        Settings
      </button>
    </div>

    <!-- Port Forwards -->
    <div class="border-t border-border">
      <div class="flex items-center justify-between px-3 py-2">
        <span class="text-xs font-semibold uppercase tracking-wider text-muted">Port Forwards</span>
        {#if ctx}
          <button
            type="button"
            onclick={() => push(`/c/${ctx}/port-forwards`)}
            class="p-1 rounded hover:bg-surface-hover transition-colors text-muted hover:text-fg"
            title="Manage port forwards"
            aria-label="Manage port forwards"
          >
            <LayoutList size={12} />
          </button>
        {/if}
      </div>

      {#if forwards.length > 0}
        <div class="flex flex-col pb-1">
          {#each forwards as fwd}
            <div class="flex items-center gap-2 px-3 py-1 group">
              <Circle
                size={8}
                class="shrink-0 fill-current
                  {fwd.status === 'active' ? 'text-green-500' :
                   fwd.status === 'reconnecting' ? 'text-yellow-500' :
                   'text-red-500'}"
              />
              <div class="flex-1 min-w-0">
                <div class="text-xs font-mono truncate">{fwd.localPort}→{fwd.targetName}</div>
                {#if fwd.podName && fwd.podName !== fwd.targetName}
                  <div class="text-xs text-muted truncate">{fwd.podName}</div>
                {/if}
              </div>
              <button
                type="button"
                onclick={() => stopForward(fwd.id)}
                class="p-0.5 rounded text-muted hover:text-fg opacity-0 group-hover:opacity-100 transition-opacity shrink-0"
                title="Stop"
                aria-label="Stop port forward {fwd.id}"
              >
                <X size={10} />
              </button>
            </div>
          {/each}
        </div>
      {:else if ctx}
        <p class="text-xs text-muted px-3 pb-2">No active forwards</p>
      {/if}
    </div>
  </div>
  {#if !sessionStore.sidebarCollapsed}
    <SidebarResizeHandle />
  {/if}
</aside>

{#if sessionStore.sidebarCollapsed}
  <button
    type="button"
    onclick={() => sessionStore.toggleSidebar()}
    class="absolute left-0 top-16 p-1.5 bg-surface border border-border border-l-0 rounded-r hover:bg-surface-hover transition-colors z-10"
    aria-label="Expand sidebar"
  >
    <PanelLeft size={14} />
  </button>
{/if}
