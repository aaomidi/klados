import {Events} from "@wailsio/runtime";
import {GetResolvedPrefs} from "$api/github.com/Vilsol/klados/internal/services/configservice.js";
import {getLogger} from "$lib/logger";

const log = getLogger("preferences");

export interface ResourceReqs {
  requests?: Record<string, string>;
  limits?: Record<string, string>;
}

export interface VolumeBrowserConfig {
  image?: string;
  mountPath?: string;
  readOnly?: boolean | null;
  activeDeadlineSeconds?: number | null;
  resources?: ResourceReqs | null;
  nodeSelector?: Record<string, string>;
  tolerations?: Record<string, unknown>[];
  promptBeforeSpawn?: boolean | null;
  orphanCleanupOnStartup?: string;
}

export interface ResolvedPrefs {
  theme: string;
  accentColor: string;
  fontSize: number;
  compactRows: boolean;
  readOnly: boolean;
  terminalWebGL: boolean;
  keybindings: Record<string, string> | null;
  savedFilters: Record<string, SavedFilter[]> | null;
  favoriteNamespaces: string[] | null;
  contextualAutocomplete: boolean;
  volumeBrowser: VolumeBrowserConfig;
}

export interface SavedFilter {
  name: string;
  labels?: Record<string, string>;
  annotations?: Record<string, string>;
  search?: string;
}

class PreferencesStore {
  prefs = $state<ResolvedPrefs>({
    theme: "system",
    accentColor: "",
    fontSize: 0,
    compactRows: false,
    readOnly: false,
    terminalWebGL: false,
    keybindings: null,
    savedFilters: null,
    favoriteNamespaces: null,
    contextualAutocomplete: true,
    volumeBrowser: {
      image: "alpine:edge",
      mountPath: "/mnt/volume",
      orphanCleanupOnStartup: "prompt",
    },
  });

  private activeContext = "";
  private unsub: (() => void) | undefined;

  async load(ctxName: string) {
    this.activeContext = ctxName;
    try {
      const resolved = await GetResolvedPrefs(ctxName);
      if (resolved) {
        this.prefs = resolved as ResolvedPrefs;
      }
    } catch (e) {
      log.error("Failed to load preferences", {error: String(e)});
    }
  }

  subscribe() {
    this.unsub = Events.On("config:updated", () => {
      this.load(this.activeContext);
    });
  }

  destroy() {
    this.unsub?.();
  }

  getKeybinding(actionID: string): string | undefined {
    return this.prefs.keybindings?.[actionID];
  }

  getSavedFilters(gvr: string): SavedFilter[] {
    return this.prefs.savedFilters?.[gvr] ?? [];
  }
}

export const preferencesStore = new PreferencesStore();
