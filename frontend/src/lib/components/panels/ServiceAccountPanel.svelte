<script lang="ts">
  import {ListResources} from "$api/github.com/Vilsol/klados/internal/services/resourceservice.js";
  import {SectionHeader, EmptyState, StatusBadge} from "@klados/ui";
  import type {KubernetesResource} from "$lib/types";

  let {obj, ctxName}: {obj: Record<string, KubernetesResource>; ctxName: string} = $props();

  const saName = $derived<string>(obj.metadata?.name ?? "");
  const saNamespace = $derived<string>(obj.metadata?.namespace ?? "");
  const automount = $derived<boolean>(obj.automountServiceAccountToken ?? true);
  const secrets = $derived<KubernetesResource[]>(obj.secrets ?? []);
  const imagePullSecrets = $derived<KubernetesResource[]>(obj.imagePullSecrets ?? []);

  interface BindingRef {
    kind: string;
    name: string;
  }
  let associatedBindings = $state<BindingRef[]>([]);

  $effect(() => {
    const name = saName;
    const ns = saNamespace;
    const ctx = ctxName;
    (async () => {
      try {
        const [rbList, crbList] = await Promise.all([
          ListResources(ctx, "rbac.authorization.k8s.io.v1.rolebindings", ns),
          ListResources(ctx, "rbac.authorization.k8s.io.v1.clusterrolebindings", ""),
        ]);
        const results: BindingRef[] = [];
        for (const binding of [...(rbList ?? []), ...(crbList ?? [])]) {
          const b = binding as KubernetesResource;
          const matched = (b.subjects ?? []).some(
            (s: KubernetesResource) => s.kind === "ServiceAccount" && s.name === name && s.namespace === ns,
          );
          if (matched) {
            results.push({kind: b.kind ?? "", name: b.metadata?.name ?? ""});
          }
        }
        associatedBindings = results;
      } catch {
        // ignore
      }
    })();
  });
</script>

<div class="p-4 space-y-6">
  <section>
    <SectionHeader>Automount Token</SectionHeader>
    <StatusBadge status={automount} mode="pill">{automount ? 'Enabled' : 'Disabled'}</StatusBadge>
  </section>

  <section>
    <SectionHeader>Secrets</SectionHeader>
    {#if secrets.length === 0}
      <EmptyState message="No secrets" />
    {:else}
      <div class="border border-border rounded overflow-hidden">
        <table class="w-full text-xs">
          <thead class="bg-surface">
            <tr>
              <th class="px-3 py-2 text-left font-medium text-muted">Name</th>
            </tr>
          </thead>
          <tbody>
            {#each secrets as s}
              <tr class="border-t border-border">
                <td class="px-3 py-2 font-medium">{s.name ?? ''}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
  </section>

  <section>
    <SectionHeader>Image Pull Secrets</SectionHeader>
    {#if imagePullSecrets.length === 0}
      <EmptyState message="No image pull secrets" />
    {:else}
      <div class="border border-border rounded overflow-hidden">
        <table class="w-full text-xs">
          <thead class="bg-surface">
            <tr>
              <th class="px-3 py-2 text-left font-medium text-muted">Name</th>
            </tr>
          </thead>
          <tbody>
            {#each imagePullSecrets as s}
              <tr class="border-t border-border">
                <td class="px-3 py-2 font-medium">{s.name ?? ''}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
  </section>

  <section>
    <SectionHeader>Associated Bindings</SectionHeader>
    {#if associatedBindings.length === 0}
      <EmptyState message="No bindings" />
    {:else}
      <div class="border border-border rounded overflow-hidden">
        <table class="w-full text-xs">
          <thead class="bg-surface">
            <tr>
              <th class="px-3 py-2 text-left font-medium text-muted w-36">Kind</th>
              <th class="px-3 py-2 text-left font-medium text-muted">Name</th>
            </tr>
          </thead>
          <tbody>
            {#each associatedBindings as b}
              <tr class="border-t border-border">
                <td class="px-3 py-2 text-muted">{b.kind}</td>
                <td class="px-3 py-2 font-medium">{b.name}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
  </section>
</div>
