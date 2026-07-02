import {Logger} from "tslog";
import {LogFrontend} from "$api/github.com/Vilsol/klados/internal/services/appservice.js";

const positionalKeyPattern = /^\d+$/;

const levelMap: Record<number, string> = {
  0: "debug", // silly
  1: "debug", // trace
  2: "debug", // debug
  3: "info",
  4: "warn",
  5: "error",
  6: "error", // fatal
};

function wailsTransport(logObj: Record<string, unknown>): void {
  const meta = logObj._meta as Record<string, unknown> | undefined;
  const level = levelMap[(meta?.logLevelId as number | undefined) ?? 3] ?? "info";

  // Positional arg '0' is the message
  const rawMsg = String(logObj["0"] ?? "");
  const name: string | undefined = meta?.name as string | undefined;
  const message = name ? `[${name}] ${rawMsg}` : rawMsg;

  // Collect structured attrs — flatten object-valued positional args
  const attrs: Record<string, unknown> = {};
  for (const [k, v] of Object.entries(logObj)) {
    if (k === "0" || k === "_meta") {
      continue;
    }
    if (positionalKeyPattern.test(k) && typeof v === "object" && v !== null && !Array.isArray(v)) {
      Object.assign(attrs, v);
    } else {
      attrs[k] = v;
    }
  }

  let attrsJSON = "";
  if (Object.keys(attrs).length > 0) {
    try {
      attrsJSON = JSON.stringify(attrs);
    } catch {
      attrsJSON = JSON.stringify({serializationError: true});
    }
  }

  LogFrontend(level, message, attrsJSON).catch(() => {
    /* empty */
  });
}

const rootLogger = new Logger({
  type: "hidden",
  name: "klados",
});

rootLogger.attachTransport(wailsTransport);

export const log = rootLogger;

export function getLogger(name: string) {
  return rootLogger.getSubLogger({name});
}
