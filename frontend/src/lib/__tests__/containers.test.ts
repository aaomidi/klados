import {describe, it, expect} from "vitest";
import {
  groupContainers,
  containerStateInfo,
  lastExit,
  decodeExitCode,
  podDiagnosis,
  probeSummaries,
  envSource,
  envFromSources,
  mountSource,
  resourceSummary,
} from "$lib/kubernetes/containers";

describe("groupContainers", () => {
  it("splits native sidecars (init with restartPolicy Always) from init containers", () => {
    const spec = {
      containers: [{name: "app"}],
      initContainers: [
        {name: "migrate"},
        {name: "proxy", restartPolicy: "Always"},
      ],
      ephemeralContainers: [{name: "debugger"}],
    };
    const g = groupContainers(spec);
    expect(g.containers.map((c) => c.name)).toEqual(["app"]);
    expect(g.sidecars.map((c) => c.name)).toEqual(["proxy"]);
    expect(g.init.map((c) => c.name)).toEqual(["migrate"]);
    expect(g.ephemeral.map((c) => c.name)).toEqual(["debugger"]);
  });

  it("handles missing spec", () => {
    const g = groupContainers(undefined);
    expect(g.containers).toEqual([]);
    expect(g.sidecars).toEqual([]);
    expect(g.init).toEqual([]);
    expect(g.ephemeral).toEqual([]);
  });
});

describe("containerStateInfo", () => {
  it("reports running with startedAt", () => {
    const info = containerStateInfo({state: {running: {startedAt: "2026-07-14T00:00:00Z"}}});
    expect(info.kind).toBe("running");
    expect(info.label).toBe("Running");
    expect(info.since).toBe("2026-07-14T00:00:00Z");
  });

  it("reports waiting with reason and message", () => {
    const info = containerStateInfo({
      state: {waiting: {reason: "CrashLoopBackOff", message: "back-off 5m0s restarting failed container"}},
    });
    expect(info.kind).toBe("waiting");
    expect(info.label).toBe("CrashLoopBackOff");
    expect(info.message).toBe("back-off 5m0s restarting failed container");
  });

  it("reports terminated with reason and exit code", () => {
    const info = containerStateInfo({state: {terminated: {reason: "Completed", exitCode: 0}}});
    expect(info.kind).toBe("terminated");
    expect(info.label).toBe("Completed");
    expect(info.exitCode).toBe(0);
  });

  it("reports unknown without status", () => {
    expect(containerStateInfo(undefined).kind).toBe("unknown");
  });
});

describe("lastExit", () => {
  it("extracts lastState.terminated", () => {
    const lx = lastExit({
      lastState: {terminated: {reason: "OOMKilled", exitCode: 137, finishedAt: "2026-07-14T10:00:00Z"}},
    });
    expect(lx).toEqual({reason: "OOMKilled", exitCode: 137, finishedAt: "2026-07-14T10:00:00Z"});
  });

  it("returns null when there was no previous termination", () => {
    expect(lastExit({state: {running: {}}})).toBeNull();
    expect(lastExit(undefined)).toBeNull();
  });
});

describe("probeSummaries", () => {
  it("summarizes http, tcp, exec and grpc probes", () => {
    const c = {
      livenessProbe: {httpGet: {path: "/healthz", port: 8080}},
      readinessProbe: {tcpSocket: {port: 5432}},
      startupProbe: {exec: {command: ["cat", "/ready"]}},
    };
    const probes = probeSummaries(c);
    expect(probes).toEqual([
      {kind: "liveness", text: "HTTP :8080/healthz"},
      {kind: "readiness", text: "TCP :5432"},
      {kind: "startup", text: "exec"},
    ]);
    expect(probeSummaries({livenessProbe: {grpc: {port: 9090}}})).toEqual([{kind: "liveness", text: "gRPC :9090"}]);
  });

  it("returns empty array without probes", () => {
    expect(probeSummaries({})).toEqual([]);
  });
});

describe("envSource", () => {
  it("returns null for plain values", () => {
    expect(envSource({name: "MODE", value: "prod"})).toBeNull();
  });

  it("resolves secretKeyRef with link", () => {
    expect(envSource({name: "PASS", valueFrom: {secretKeyRef: {name: "db-creds", key: "password"}}})).toEqual({
      text: "secret/db-creds › password",
      gvr: "core.v1.secrets",
      name: "db-creds",
    });
  });

  it("resolves configMapKeyRef with link", () => {
    expect(envSource({name: "URL", valueFrom: {configMapKeyRef: {name: "app-config", key: "url"}}})).toEqual({
      text: "configmap/app-config › url",
      gvr: "core.v1.configmaps",
      name: "app-config",
    });
  });

  it("resolves fieldRef and resourceFieldRef as text only", () => {
    expect(envSource({name: "IP", valueFrom: {fieldRef: {fieldPath: "status.podIP"}}})).toEqual({text: "field status.podIP"});
    expect(envSource({name: "MEM", valueFrom: {resourceFieldRef: {resource: "limits.memory"}}})).toEqual({
      text: "resource limits.memory",
    });
  });
});

describe("envFromSources", () => {
  it("resolves configMapRef and secretRef", () => {
    const c = {envFrom: [{configMapRef: {name: "common"}}, {secretRef: {name: "creds"}, prefix: "DB_"}]};
    expect(envFromSources(c)).toEqual([
      {text: "configmap/common", gvr: "core.v1.configmaps", name: "common"},
      {text: "DB_* from secret/creds", gvr: "core.v1.secrets", name: "creds"},
    ]);
  });
});

describe("mountSource", () => {
  const volumes = [
    {name: "data", persistentVolumeClaim: {claimName: "pg-data"}},
    {name: "cfg", configMap: {name: "app-config"}},
    {name: "tls", secret: {secretName: "tls-cert"}},
    {name: "tmp", emptyDir: {}},
    {name: "host", hostPath: {path: "/var/log"}},
  ];

  it("resolves pvc, configmap and secret volumes with links", () => {
    expect(mountSource({name: "data", mountPath: "/var/lib/pg"}, volumes)).toEqual({
      text: "pvc/pg-data",
      gvr: "core.v1.persistentvolumeclaims",
      name: "pg-data",
    });
    expect(mountSource({name: "cfg", mountPath: "/etc/app"}, volumes)).toEqual({
      text: "configmap/app-config",
      gvr: "core.v1.configmaps",
      name: "app-config",
    });
    expect(mountSource({name: "tls", mountPath: "/etc/tls"}, volumes)).toEqual({
      text: "secret/tls-cert",
      gvr: "core.v1.secrets",
      name: "tls-cert",
    });
  });

  it("resolves non-linkable volumes as text", () => {
    expect(mountSource({name: "tmp", mountPath: "/tmp"}, volumes)).toEqual({text: "emptyDir"});
    expect(mountSource({name: "host", mountPath: "/logs"}, volumes)).toEqual({text: "hostPath /var/log"});
  });

  it("falls back to the mount name when the volume is unknown", () => {
    expect(mountSource({name: "ghost", mountPath: "/x"}, volumes)).toEqual({text: "ghost"});
  });
});

describe("resourceSummary", () => {
  it("formats req/limit pairs", () => {
    expect(
      resourceSummary({requests: {cpu: "100m", memory: "128Mi"}, limits: {cpu: "500m", memory: "512Mi"}}),
    ).toBe("CPU 100m / 500m · Mem 128Mi / 512Mi");
  });

  it("shows dashes for missing sides and includes ephemeral storage as Disk", () => {
    expect(resourceSummary({limits: {"ephemeral-storage": "1Gi"}})).toBe("Disk — / 1Gi");
  });

  it("returns empty string without resources", () => {
    expect(resourceSummary(undefined)).toBe("");
    expect(resourceSummary({})).toBe("");
  });
});

describe("decodeExitCode", () => {
  it("decodes 137 as OOM when reason says so", () => {
    expect(decodeExitCode(137, "OOMKilled")).toBe("SIGKILL (out of memory)");
  });
  it("decodes 137 generically without reason", () => {
    expect(decodeExitCode(137)).toBe("SIGKILL — OOM or forced kill");
  });
  it("decodes common codes", () => {
    expect(decodeExitCode(143)).toBe("SIGTERM — graceful shutdown");
    expect(decodeExitCode(139)).toBe("SIGSEGV — segmentation fault");
    expect(decodeExitCode(127)).toBe("command not found");
    expect(decodeExitCode(126)).toBe("command not executable");
    expect(decodeExitCode(1)).toBe("application error");
    expect(decodeExitCode(0)).toBe("success");
  });
  it("decodes other signals generically", () => {
    expect(decodeExitCode(130)).toBe("signal 2");
  });
  it("falls back to exit N", () => {
    expect(decodeExitCode(2)).toBe("exit 2");
  });
});

describe("podDiagnosis", () => {
  it("hides for a healthy running pod", () => {
    const obj = {status: {phase: "Running", containerStatuses: [
      {name: "app", ready: true, state: {running: {}}},
    ]}};
    expect(podDiagnosis(obj).show).toBe(false);
  });
  it("hides for a completed pod", () => {
    const obj = {status: {phase: "Succeeded", containerStatuses: [
      {name: "app", ready: false, state: {terminated: {reason: "Completed", exitCode: 0}}},
    ]}};
    expect(podDiagnosis(obj).show).toBe(false);
  });
  it("shows pod-level eviction reason and message", () => {
    const obj = {status: {phase: "Failed", reason: "Evicted", message: "low on memory"}};
    const d = podDiagnosis(obj);
    expect(d.show).toBe(true);
    expect(d.reason).toBe("Evicted");
    expect(d.message).toBe("low on memory");
  });
  it("counts unhealthy containers", () => {
    const obj = {status: {phase: "Running", containerStatuses: [
      {name: "app", ready: false, state: {waiting: {reason: "CrashLoopBackOff"}}},
      {name: "side", ready: true, state: {running: {}}},
    ]}};
    const d = podDiagnosis(obj);
    expect(d.show).toBe(true);
    expect(d.unhealthy).toBe(1);
    expect(d.total).toBe(2);
  });
  it("ignores ContainerCreating as unhealthy", () => {
    const obj = {status: {phase: "Pending", containerStatuses: [
      {name: "app", ready: false, state: {waiting: {reason: "ContainerCreating"}}},
    ]}};
    expect(podDiagnosis(obj).show).toBe(false);
  });
  it("skips native sidecars but flags failed init", () => {
    const obj = {status: {phase: "Pending", initContainerStatuses: [
      {name: "proxy", started: true, ready: true, state: {running: {}}},
      {name: "migrate", state: {terminated: {reason: "Error", exitCode: 1}}},
    ]}};
    expect(podDiagnosis(obj).show).toBe(true);
  });
});
