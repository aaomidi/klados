import {Events} from "@wailsio/runtime";
import {ListForwards} from "$api/github.com/Vilsol/klados/internal/services/portforwardservice.js";

export interface ForwardSpec {
  id: string;
  contextName: string;
  namespace: string;
  targetKind: string;
  targetName: string;
  targetGVR: string;
  localPort: number;
  remotePort: number;
  status: string;
  podName: string;
  error: string;
}

class PortForwardStore {
  // forwards keyed by context name → list of active specs
  byContext = $state<Record<string, ForwardSpec[]>>({});

  // Tracks subscribed contexts so we don't double-subscribe.
  private subscriptions = new Map<string, () => void>();

  async subscribe(ctxName: string): Promise<() => void> {
    if (!ctxName) return () => {};

    if (this.subscriptions.has(ctxName)) {
      const refcount = (this.refcounts[ctxName] ?? 0) + 1;
      this.refcounts[ctxName] = refcount;
      return () => this.release(ctxName);
    }

    this.refcounts[ctxName] = 1;
    await this.reload(ctxName);
    const unsub = Events.On(`portforward:${ctxName}:updated`, () => {
      this.reload(ctxName);
    });
    this.subscriptions.set(ctxName, unsub);
    return () => this.release(ctxName);
  }

  private refcounts: Record<string, number> = {};

  private release(ctxName: string) {
    const remaining = (this.refcounts[ctxName] ?? 1) - 1;
    if (remaining > 0) {
      this.refcounts[ctxName] = remaining;
      return;
    }
    delete this.refcounts[ctxName];
    const unsub = this.subscriptions.get(ctxName);
    if (unsub) {
      unsub();
      this.subscriptions.delete(ctxName);
    }
    const next = {...this.byContext};
    delete next[ctxName];
    this.byContext = next;
  }

  private async reload(ctxName: string) {
    try {
      const result = await ListForwards(ctxName);
      this.byContext = {...this.byContext, [ctxName]: (result ?? []) as ForwardSpec[]};
    } catch {
      // ignore
    }
  }

  /** Active forwards targeting the given pod (matches direct pod targets and selector targets that resolved to it). */
  forPod(ctxName: string, namespace: string, podName: string): ForwardSpec[] {
    const list = this.byContext[ctxName] ?? [];
    return list.filter((f) => {
      if (f.namespace !== namespace) return false;
      if (f.targetKind === "pod" || f.targetKind === "statefulpod") {
        return f.targetName === podName;
      }
      return f.podName === podName;
    });
  }
}

export const portForwardStore = new PortForwardStore();
