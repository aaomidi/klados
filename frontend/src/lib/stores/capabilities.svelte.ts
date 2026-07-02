import {GetCapabilities} from "$api/github.com/Vilsol/klados/internal/services/appservice.js";

export interface Capabilities {
  portForwarding: boolean;
  osWindows: boolean;
  nativeDialogs: boolean;
  mode: string;
}

/**
 * What this deployment supports. Desktop mode exposes native dialogs, real
 * OS pop-out windows, and port-forwarding; the hosted server does not, and
 * the UI falls back to web equivalents (file inputs, window.open) or hides
 * the feature. Defaults are the conservative web set until the RPC resolves.
 */
class CapabilitiesStore {
  portForwarding = $state(false);
  osWindows = $state(false);
  nativeDialogs = $state(false);
  mode = $state("");
  loaded = $state(false);

  constructor() {
    if (typeof window !== "undefined") {
      GetCapabilities()
        .then((caps) => {
          this.portForwarding = !!caps?.portForwarding;
          this.osWindows = !!caps?.osWindows;
          this.nativeDialogs = !!caps?.nativeDialogs;
          this.mode = caps?.mode ?? "";
          this.loaded = true;
        })
        .catch(() => {
          this.loaded = true;
        });
    }
  }
}

export const capabilitiesStore = new CapabilitiesStore();
