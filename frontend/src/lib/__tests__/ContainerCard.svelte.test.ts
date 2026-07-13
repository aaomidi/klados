import {describe, it, expect, vi} from "vitest";
import {render, screen, fireEvent} from "@testing-library/svelte";
import ContainerCard from "$lib/components/panels/ContainerCard.svelte";

const runningContainer = {
  name: "app",
  image: "registry.example.com/app:1.2.3",
  resources: {requests: {cpu: "100m", memory: "128Mi"}, limits: {cpu: "500m", memory: "512Mi"}},
  livenessProbe: {httpGet: {path: "/healthz", port: 8080}},
  env: [
    {name: "MODE", value: "prod"},
    {name: "PASS", valueFrom: {secretKeyRef: {name: "db-creds", key: "password"}}},
  ],
  volumeMounts: [{name: "data", mountPath: "/var/lib/pg"}],
};

const runningStatus = {
  name: "app",
  ready: true,
  restartCount: 0,
  state: {running: {startedAt: new Date(Date.now() - 3 * 3600 * 1000).toISOString()}},
};

const volumes = [{name: "data", persistentVolumeClaim: {claimName: "pg-data"}}];

describe("ContainerCard", () => {
  it("shows uptime for running containers", () => {
    render(ContainerCard, {props: {container: runningContainer, status: runningStatus}});
    expect(screen.getByText(/up 3h/)).toBeTruthy();
    expect(screen.getByText("Running")).toBeTruthy();
  });

  it("shows waiting reason, message and last exit forensics for a crashlooping container", () => {
    const status = {
      name: "app",
      ready: false,
      restartCount: 7,
      state: {waiting: {reason: "CrashLoopBackOff", message: "back-off 5m0s restarting failed container"}},
      lastState: {terminated: {reason: "OOMKilled", exitCode: 137, finishedAt: new Date(Date.now() - 240 * 1000).toISOString()}},
    };
    render(ContainerCard, {props: {container: runningContainer, status}});
    expect(screen.getByText("CrashLoopBackOff")).toBeTruthy();
    expect(screen.getByText(/back-off 5m0s restarting failed container/)).toBeTruthy();
    expect(screen.getByText(/Last exit: OOMKilled \(exit 137\)/)).toBeTruthy();
    expect(screen.getByText(/7 restarts/)).toBeTruthy();
  });

  it("renders a probes line", () => {
    render(ContainerCard, {props: {container: runningContainer, status: runningStatus}});
    expect(screen.getByText(/liveness HTTP :8080\/healthz/)).toBeTruthy();
  });

  it("renders resource summary on one line", () => {
    render(ContainerCard, {props: {container: runningContainer, status: runningStatus}});
    expect(screen.getByText("CPU 100m / 500m · Mem 128Mi / 512Mi")).toBeTruthy();
  });

  it("links env valueFrom sources through onopenresource", async () => {
    const onopenresource = vi.fn();
    render(ContainerCard, {
      props: {container: runningContainer, status: runningStatus, namespace: "prod", onopenresource},
    });
    await fireEvent.click(screen.getByText(/2 env vars/));
    await fireEvent.click(screen.getByText("secret/db-creds › password"));
    expect(onopenresource).toHaveBeenCalledWith("core.v1.secrets", "prod", "db-creds");
  });

  it("links mount volume sources through onopenresource", async () => {
    const onopenresource = vi.fn();
    render(ContainerCard, {
      props: {container: runningContainer, status: runningStatus, volumes, namespace: "prod", onopenresource},
    });
    await fireEvent.click(screen.getByText(/1 mount/));
    await fireEvent.click(screen.getByText("pvc/pg-data"));
    expect(onopenresource).toHaveBeenCalledWith("core.v1.persistentvolumeclaims", "prod", "pg-data");
  });

  it("offers Logs and Shell shortcuts via onopencontainer", async () => {
    const onopencontainer = vi.fn();
    render(ContainerCard, {props: {container: runningContainer, status: runningStatus, onopencontainer}});
    await fireEvent.click(screen.getByRole("button", {name: "Logs"}));
    expect(onopencontainer).toHaveBeenCalledWith("logs", "app");
    await fireEvent.click(screen.getByRole("button", {name: "Shell"}));
    expect(onopencontainer).toHaveBeenCalledWith("terminal", "app");
  });

  it("hides the Shell shortcut for containers that are not running", () => {
    const onopencontainer = vi.fn();
    const status = {
      name: "app",
      ready: false,
      state: {terminated: {reason: "Error", exitCode: 1}},
    };
    render(ContainerCard, {props: {container: runningContainer, status, onopencontainer}});
    expect(screen.queryByRole("button", {name: "Shell"})).toBeNull();
    expect(screen.getByRole("button", {name: "Logs"})).toBeTruthy();
  });

  it("hides the Shell shortcut for waiting containers", () => {
    const onopencontainer = vi.fn();
    const status = {
      name: "app",
      ready: false,
      state: {waiting: {reason: "CrashLoopBackOff"}},
    };
    render(ContainerCard, {props: {container: runningContainer, status, onopencontainer}});
    expect(screen.queryByRole("button", {name: "Shell"})).toBeNull();
  });

  it("compact variant shows state and exit code without detail rows", () => {
    const status = {
      name: "migrate",
      ready: false,
      state: {terminated: {reason: "Completed", exitCode: 0}},
    };
    render(ContainerCard, {
      props: {container: {name: "migrate", image: "migrator:2"}, status, compact: true},
    });
    expect(screen.getByText(/Completed \(exit 0\)/)).toBeTruthy();
    expect(screen.queryByText(/env vars/)).toBeNull();
  });
});
