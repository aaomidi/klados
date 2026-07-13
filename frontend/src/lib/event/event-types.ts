import type {KubernetesResource} from "$lib/types";

export type Severity = "Warning" | "Normal";

export interface InvolvedObjectRef {
  kind: string;
  apiVersion: string;
  name: string;
  namespace: string;
  uid: string;
}

export interface EventItem {
  metadata?: {name?: string; namespace?: string; uid?: string; creationTimestamp?: string};
  type?: string;
  reason?: string;
  message?: string;
  count?: number;
  firstTimestamp?: string;
  lastTimestamp?: string;
  eventTime?: string;
  involvedObject?: Partial<InvolvedObjectRef>;
  related?: Partial<InvolvedObjectRef>;
  source?: {component?: string; host?: string};
  reportingController?: string;
  [k: string]: KubernetesResource | undefined;
}

export interface GroupedEvent {
  key: string;
  reason: string;
  involvedObject: InvolvedObjectRef;
  severity: Severity;
  count: number;
  firstSeen: string;
  lastSeen: string;
  message: string;
  sample: EventItem; // the most recent contributing event
}

export type EventRow = EventItem | GroupedEvent;

export function isGrouped(row: EventRow): row is GroupedEvent {
  return (row as GroupedEvent).key !== undefined;
}
