export interface MessageRef {
  start: number;
  end: number;
  name: string;
  gvr: string;
  clusterScoped: boolean;
}

// Well-known controller message shapes. Each regex captures the referenced
// object name in group 1; the /d flag gives exact capture offsets.
const PATTERNS: Array<{re: RegExp; gvr: string; clusterScoped?: boolean}> = [
  {re: /\b(?:Created|Deleted) pod: ?([a-z0-9][a-z0-9.-]*)/gid, gvr: "core.v1.pods"},
  {re: /\b(?:create|delete) Pod ([a-z0-9][a-z0-9.-]*) in StatefulSet/gd, gvr: "core.v1.pods"},
  {re: /\bScaled (?:up|down) replica set (?:[a-z0-9.-]+\/)?([a-z0-9][a-z0-9.-]*)/gid, gvr: "apps.v1.replicasets"},
  {re: /\b(?:Created|Deleted|Saw completed) job:? ([a-z0-9][a-z0-9.-]*)/gid, gvr: "batch.v1.jobs"},
  {re: /\bSuccessfully assigned [a-z0-9.-]+\/[a-z0-9.-]+ to ([a-z0-9][a-z0-9.-]*)/gd, gvr: "core.v1.nodes", clusterScoped: true},
];

export function extractMessageRefs(message: string): MessageRef[] {
  if (!message) return [];
  const refs: MessageRef[] = [];
  for (const {re, gvr, clusterScoped} of PATTERNS) {
    re.lastIndex = 0;
    for (const m of message.matchAll(re)) {
      const indices = (m as RegExpMatchArray & {indices?: Array<[number, number]>}).indices?.[1];
      if (!indices) continue;
      let [start, end] = indices;
      let name = m[1];
      // valid k8s names never end in punctuation — trim sentence trailers
      while (name && /[.-]$/.test(name)) {
        name = name.slice(0, -1);
        end--;
      }
      if (name) refs.push({start, end, name, gvr, clusterScoped: clusterScoped ?? false});
    }
  }
  refs.sort((a, b) => a.start - b.start);
  return refs.filter((r, i) => i === 0 || r.start >= refs[i - 1].end);
}
