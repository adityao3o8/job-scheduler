import { NextResponse } from "next/server";

export function json(data: unknown, status = 200) {
  return NextResponse.json(data, { status });
}

export function error(message: string, status = 400, code = "ERROR") {
  return json({ error: message, code }, status);
}

export function unauthorized(message = "unauthorized") {
  return error(message, 401, "UNAUTHORIZED");
}

export function toIso(v: unknown): string | undefined {
  if (v == null) return undefined;
  if (v instanceof Date) return v.toISOString();
  return String(v);
}

export function mapQueue(row: Record<string, unknown>) {
  return {
    id: row.id,
    project_id: row.project_id,
    name: row.name,
    slug: row.slug,
    retry_policy_id: row.retry_policy_id ?? undefined,
    priority_default: Number(row.priority_default ?? 0),
    concurrency_limit: row.concurrency_limit != null ? Number(row.concurrency_limit) : undefined,
    is_paused: Boolean(row.paused),
    created_at: toIso(row.created_at),
    updated_at: toIso(row.updated_at),
  };
}

export function mapJob(row: Record<string, unknown>) {
  return {
    id: row.id,
    queue_id: row.queue_id,
    status: row.status,
    priority: Number(row.priority ?? 0),
    payload: typeof row.payload === "string" ? JSON.parse(row.payload) : row.payload,
    idempotency_key: row.idempotency_key ?? undefined,
    next_run_at: toIso(row.next_run_at),
    attempts: Number(row.attempts ?? 0),
    max_attempts: Number(row.max_attempts ?? 3),
    retry_policy_id: row.retry_policy_id ?? undefined,
    worker_id: row.worker_id ?? undefined,
    claimed_at: toIso(row.claimed_at),
    lease_expires_at: toIso(row.lease_expires_at),
    completed_at: toIso(row.completed_at),
    failed_at: toIso(row.failed_at),
    error_message: row.error_message ?? undefined,
    cron_expr: row.cron_expr ?? undefined,
    batch_id: row.batch_id ?? undefined,
    created_at: toIso(row.created_at),
    updated_at: toIso(row.updated_at),
  };
}
