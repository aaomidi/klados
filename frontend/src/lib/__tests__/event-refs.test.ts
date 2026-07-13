import {describe, it, expect} from "vitest";
import {extractMessageRefs} from "$lib/event/event-refs";

describe("extractMessageRefs", () => {
  it("extracts pod from 'Created pod:' messages", () => {
    const refs = extractMessageRefs("Created pod: web-6d4cf56db6-x7k2p");
    expect(refs).toEqual([
      {start: 13, end: 33, name: "web-6d4cf56db6-x7k2p", gvr: "core.v1.pods", clusterScoped: false},
    ]);
  });

  it("extracts pod from 'Deleted pod:' messages", () => {
    const refs = extractMessageRefs("Deleted pod: web-6d4cf56db6-x7k2p");
    expect(refs).toHaveLength(1);
    expect(refs[0].name).toBe("web-6d4cf56db6-x7k2p");
    expect(refs[0].gvr).toBe("core.v1.pods");
  });

  it("extracts replica set from deployment scale messages", () => {
    const refs = extractMessageRefs("Scaled up replica set web-6d4cf56db6 from 0 to 1");
    expect(refs).toHaveLength(1);
    expect(refs[0].name).toBe("web-6d4cf56db6");
    expect(refs[0].gvr).toBe("apps.v1.replicasets");
  });

  it("extracts replica set from namespaced scale messages (k8s >= 1.28)", () => {
    const refs = extractMessageRefs("Scaled down replica set default/web-6d4cf56db6 from 1 to 0");
    expect(refs).toHaveLength(1);
    expect(refs[0].name).toBe("web-6d4cf56db6");
  });

  it("extracts pod from statefulset messages", () => {
    const refs = extractMessageRefs("create Pod db-0 in StatefulSet db successful");
    expect(refs).toHaveLength(1);
    expect(refs[0].name).toBe("db-0");
    expect(refs[0].gvr).toBe("core.v1.pods");
  });

  it("extracts job from cronjob messages", () => {
    const refs = extractMessageRefs("Created job backup-29012345");
    expect(refs).toHaveLength(1);
    expect(refs[0].name).toBe("backup-29012345");
    expect(refs[0].gvr).toBe("batch.v1.jobs");
  });

  it("extracts node from scheduler assignment messages as cluster-scoped", () => {
    const refs = extractMessageRefs("Successfully assigned default/web-abc to ip-10-0-0-1.ec2.internal");
    expect(refs).toHaveLength(1);
    expect(refs[0].name).toBe("ip-10-0-0-1.ec2.internal");
    expect(refs[0].gvr).toBe("core.v1.nodes");
    expect(refs[0].clusterScoped).toBe(true);
  });

  it("strips trailing punctuation from captured names", () => {
    const refs = extractMessageRefs("Saw completed job: backup-29012345, status: Complete");
    expect(refs).toHaveLength(1);
    expect(refs[0].name).toBe("backup-29012345");
  });

  it("returns refs with correct offsets for slicing the message", () => {
    const msg = "Scaled up replica set web-6d4cf56db6 from 0 to 1";
    const [ref] = extractMessageRefs(msg);
    expect(msg.slice(ref.start, ref.end)).toBe("web-6d4cf56db6");
  });

  it("returns empty array for messages without references", () => {
    expect(extractMessageRefs("Back-off restarting failed container")).toEqual([]);
    expect(extractMessageRefs("")).toEqual([]);
  });

  it("does not false-positive on 'Pod sandbox changed' kubelet messages", () => {
    expect(extractMessageRefs("Pod sandbox changed, it will be killed and re-created.")).toEqual([]);
  });
});
