export type StatusKey =
  | "queued"
  | "scheduled"
  | "claimed"
  | "running"
  | "completed"
  | "failed"
  | "dead_letter"
  | "retrying"
  | "active"
  | "offline"
  | "dead"
  | "paused";

export interface StatusStyle {
  color: string;
  dim: string;
  label: string;
}

const STYLES: Record<string, StatusStyle> = {
  queued: { color: "var(--st-queued)", dim: "var(--st-queued-dim)", label: "Queued" },
  scheduled: { color: "var(--st-scheduled)", dim: "var(--st-scheduled-dim)", label: "Scheduled" },
  claimed: { color: "var(--st-claimed)", dim: "var(--st-claimed-dim)", label: "Claimed" },
  running: { color: "var(--st-running)", dim: "var(--st-running-dim)", label: "Running" },
  completed: { color: "var(--st-completed)", dim: "var(--st-completed-dim)", label: "Completed" },
  failed: { color: "var(--st-failed)", dim: "var(--st-failed-dim)", label: "Failed" },
  dead_letter: { color: "var(--st-dead)", dim: "var(--st-dead-dim)", label: "Dead letter" },
  dead: { color: "var(--st-dead)", dim: "var(--st-dead-dim)", label: "Dead" },
  retrying: { color: "var(--st-retrying)", dim: "var(--st-retrying-dim)", label: "Retrying" },
  active: { color: "var(--st-active)", dim: "var(--st-active-dim)", label: "Active" },
  offline: { color: "var(--st-offline)", dim: "var(--st-offline-dim)", label: "Offline" },
  paused: { color: "var(--st-paused)", dim: "var(--st-paused-dim)", label: "Paused" },
};

export function getStatusStyle(status: string): StatusStyle {
  const key = status.toLowerCase().replace(/\s+/g, "_");
  return STYLES[key] ?? {
    color: "var(--text-tertiary)",
    dim: "var(--bg-surface-2)",
    label: status,
  };
}

export const JOB_STATUSES = [
  "queued",
  "scheduled",
  "claimed",
  "running",
  "completed",
  "failed",
  "dead_letter",
] as const;
