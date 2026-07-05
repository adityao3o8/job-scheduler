export function formatRelativeTime(ts: string): string {
  const diff = Date.now() - new Date(ts).getTime();
  const secs = Math.floor(diff / 1000);
  if (secs < 5) return "just now";
  if (secs < 60) return `${secs}s ago`;
  const mins = Math.floor(secs / 60);
  if (mins < 60) return `${mins}m ago`;
  const hrs = Math.floor(mins / 60);
  if (hrs < 24) return `${hrs}h ago`;
  const days = Math.floor(hrs / 24);
  return `${days}d ago`;
}

type JobLike = {
  idempotency_key?: string;
  payload?: Record<string, unknown>;
  cron_expr?: string;
};

export function jobDisplayName(job: JobLike, fallback = "Unnamed job"): string {
  if (job.idempotency_key) return job.idempotency_key;

  const payload = job.payload;
  if (payload && typeof payload === "object") {
    const type = typeof payload.type === "string" ? payload.type : null;
    const message = typeof payload.message === "string" ? payload.message : null;
    const duration =
      typeof payload.duration_ms === "number" ? payload.duration_ms : null;

    if (type && message) return `${type} — ${message}`;
    if (type && job.cron_expr) return `${type} (recurring)`;
    if (type && duration != null) return `${type} (${duration}ms)`;
    if (type) return type;
  }

  return fallback;
}

export function queueName(
  queueId: string,
  queues: { id: string; name: string }[]
): string {
  return queues.find((q) => q.id === queueId)?.name ?? "Unknown queue";
}

export function slugifyName(name: string): string {
  return name
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "");
}

export function workerName(
  workerId: string,
  workers: { id: string; name: string }[]
): string {
  return workers.find((w) => w.id === workerId)?.name ?? "Unknown worker";
}

export function truncateId(id: string, len = 8): string {
  return id.length > len ? `${id.slice(0, len)}…` : id;
}

export function formatDuration(ms?: number): string {
  if (ms == null) return "—";
  if (ms < 1000) return `${ms}ms`;
  return `${(ms / 1000).toFixed(2)}s`;
}

export function formatPercent(rate: number): string {
  return `${(rate * 100).toFixed(1)}%`;
}
