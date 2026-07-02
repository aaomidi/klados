// @ts-nocheck
// Pop-out panels: desktop mode opens a real OS window via the WindowService
// RPC (exactly the pre-split behaviour: focus-if-exists, panel:closed on
// close); the web build falls back to a browser window. Cross-window
// coordination (panel:init/ready/pop-in/closed) rides the event hub in both.

import { window as windowSvc } from "$lib/transport/clients";
import { Code, ConnectError } from "@connectrpc/connect";

function openBrowserPanel(panelID, title) {
    const url = new URL(globalThis.location.href);
    url.search = `?panel=${encodeURIComponent(panelID)}`;
    const win = globalThis.open(url.toString(), `klados-panel-${panelID}`, "width=1000,height=600");
    if (win) {
        win.document.title = title || "Klados";
    }
}

export function OpenPanelWindow(panelID, title) {
    return windowSvc
        .openPanelWindow({ panelId: panelID, title })
        .then(() => undefined)
        .catch((err) => {
            if (err instanceof ConnectError && err.code === Code.Unimplemented) {
                openBrowserPanel(panelID, title);
                return;
            }
            throw err;
        });
}
