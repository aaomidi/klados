import type {KubernetesResource} from "$lib/types";

export interface ContainerGroups {
  containers: KubernetesResource[];
  sidecars: KubernetesResource[];
  init: KubernetesResource[];
  ephemeral: KubernetesResource[];
}

export function groupContainers(spec: KubernetesResource | undefined): ContainerGroups {
  const initAll: KubernetesResource[] = spec?.initContainers ?? [];
  return {
    containers: spec?.containers ?? [],
    sidecars: initAll.filter((c) => c.restartPolicy === "Always"),
    init: initAll.filter((c) => c.restartPolicy !== "Always"),
    ephemeral: spec?.ephemeralContainers ?? [],
  };
}

export interface StateInfo {
  kind: "running" | "waiting" | "terminated" | "unknown";
  label: string;
  message?: string;
  since?: string;
  exitCode?: number;
}

export function containerStateInfo(status: KubernetesResource | undefined): StateInfo {
  const state = status?.state;
  if (state?.running) {
    return {kind: "running", label: "Running", since: state.running.startedAt};
  }
  if (state?.waiting) {
    return {kind: "waiting", label: state.waiting.reason || "Waiting", message: state.waiting.message};
  }
  if (state?.terminated) {
    return {
      kind: "terminated",
      label: state.terminated.reason || "Terminated",
      message: state.terminated.message,
      exitCode: state.terminated.exitCode,
    };
  }
  return {kind: "unknown", label: "Unknown"};
}

export interface LastExit {
  reason: string;
  exitCode: number;
  finishedAt?: string;
}

export function lastExit(status: KubernetesResource | undefined): LastExit | null {
  const t = status?.lastState?.terminated;
  if (!t) return null;
  return {reason: t.reason || "Terminated", exitCode: t.exitCode ?? 0, finishedAt: t.finishedAt};
}

export interface ProbeInfo {
  kind: "liveness" | "readiness" | "startup";
  text: string;
}

function probeText(probe: KubernetesResource): string {
  if (probe.httpGet) {
    const scheme = probe.httpGet.scheme === "HTTPS" ? "HTTPS" : "HTTP";
    return `${scheme} :${probe.httpGet.port}${probe.httpGet.path ?? ""}`;
  }
  if (probe.tcpSocket) return `TCP :${probe.tcpSocket.port}`;
  if (probe.grpc) return `gRPC :${probe.grpc.port}`;
  if (probe.exec) return "exec";
  return "probe";
}

export function probeSummaries(c: KubernetesResource): ProbeInfo[] {
  const out: ProbeInfo[] = [];
  if (c.livenessProbe) out.push({kind: "liveness", text: probeText(c.livenessProbe)});
  if (c.readinessProbe) out.push({kind: "readiness", text: probeText(c.readinessProbe)});
  if (c.startupProbe) out.push({kind: "startup", text: probeText(c.startupProbe)});
  return out;
}

export interface RefLink {
  text: string;
  gvr?: string;
  name?: string;
}

export function envSource(e: KubernetesResource): RefLink | null {
  const vf = e.valueFrom;
  if (!vf) return null;
  if (vf.secretKeyRef) {
    return {text: `secret/${vf.secretKeyRef.name} › ${vf.secretKeyRef.key}`, gvr: "core.v1.secrets", name: vf.secretKeyRef.name};
  }
  if (vf.configMapKeyRef) {
    return {
      text: `configmap/${vf.configMapKeyRef.name} › ${vf.configMapKeyRef.key}`,
      gvr: "core.v1.configmaps",
      name: vf.configMapKeyRef.name,
    };
  }
  if (vf.fieldRef) return {text: `field ${vf.fieldRef.fieldPath}`};
  if (vf.resourceFieldRef) return {text: `resource ${vf.resourceFieldRef.resource}`};
  return {text: "(computed)"};
}

export function envFromSources(c: KubernetesResource): RefLink[] {
  return (c.envFrom ?? []).map((src: KubernetesResource): RefLink => {
    const prefix = src.prefix ? `${src.prefix}* from ` : "";
    if (src.configMapRef) {
      return {text: `${prefix}configmap/${src.configMapRef.name}`, gvr: "core.v1.configmaps", name: src.configMapRef.name};
    }
    if (src.secretRef) {
      return {text: `${prefix}secret/${src.secretRef.name}`, gvr: "core.v1.secrets", name: src.secretRef.name};
    }
    return {text: `${prefix}envFrom`};
  });
}

export function mountSource(mount: KubernetesResource, volumes: KubernetesResource[]): RefLink {
  const vol = volumes.find((v) => v.name === mount.name);
  if (!vol) return {text: mount.name ?? "volume"};
  if (vol.persistentVolumeClaim) {
    return {
      text: `pvc/${vol.persistentVolumeClaim.claimName}`,
      gvr: "core.v1.persistentvolumeclaims",
      name: vol.persistentVolumeClaim.claimName,
    };
  }
  if (vol.configMap) return {text: `configmap/${vol.configMap.name}`, gvr: "core.v1.configmaps", name: vol.configMap.name};
  if (vol.secret) return {text: `secret/${vol.secret.secretName}`, gvr: "core.v1.secrets", name: vol.secret.secretName};
  if (vol.emptyDir) return {text: "emptyDir"};
  if (vol.hostPath) return {text: `hostPath ${vol.hostPath.path}`};
  if (vol.projected) return {text: "projected"};
  if (vol.downwardAPI) return {text: "downwardAPI"};
  const kind = Object.keys(vol).find((k) => k !== "name");
  return {text: kind ?? "volume"};
}

export function resourceSummary(resources: KubernetesResource | undefined): string {
  if (!resources) return "";
  const req = resources.requests ?? {};
  const lim = resources.limits ?? {};
  const parts: string[] = [];
  const pair = (label: string, key: string) => {
    if (req[key] || lim[key]) parts.push(`${label} ${req[key] ?? "—"} / ${lim[key] ?? "—"}`);
  };
  pair("CPU", "cpu");
  pair("Mem", "memory");
  pair("Disk", "ephemeral-storage");
  return parts.join(" · ");
}

export function decodeExitCode(code: number, reason?: string): string {
  switch (code) {
    case 0:
      return "success";
    case 1:
      return "application error";
    case 126:
      return "command not executable";
    case 127:
      return "command not found";
    case 137:
      return reason === "OOMKilled" ? "SIGKILL (out of memory)" : "SIGKILL — OOM or forced kill";
    case 139:
      return "SIGSEGV — segmentation fault";
    case 143:
      return "SIGTERM — graceful shutdown";
  }
  if (code > 128 && code < 165) return `signal ${code - 128}`;
  return `exit ${code}`;
}

export interface PodDiagnosis {
  show: boolean;
  reason?: string;
  message?: string;
  unhealthy: number;
  total: number;
}

function isContainerUnhealthy(cs: KubernetesResource): boolean {
  const w = cs.state?.waiting;
  if (w?.reason && w.reason !== "ContainerCreating" && w.reason !== "PodInitializing") return true;
  const t = cs.state?.terminated;
  if (t && (t.exitCode ?? 0) !== 0) return true;
  return false;
}

export function podDiagnosis(obj: KubernetesResource | undefined): PodDiagnosis {
  const status = obj?.status ?? {};
  const statuses: KubernetesResource[] = status.containerStatuses ?? [];
  const initStatuses: KubernetesResource[] = status.initContainerStatuses ?? [];
  let unhealthy = 0;
  for (const cs of statuses) if (isContainerUnhealthy(cs)) unhealthy++;
  let initFailed = false;
  for (const cs of initStatuses) {
    if (cs.started && cs.ready) continue; // native sidecar
    if (isContainerUnhealthy(cs)) initFailed = true;
  }
  const reason: string | undefined = status.reason;
  const show = Boolean(reason) || unhealthy > 0 || initFailed;
  return {show, reason, message: status.message, unhealthy, total: statuses.length};
}
