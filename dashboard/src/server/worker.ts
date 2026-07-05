import { getSql } from "./db";

const WORKER_NAME = "vercel-worker-1";
const LEASE_SECONDS = 30;

async function ensureWorker(sql: ReturnType<typeof getSql>) {
  const existing = await sql`SELECT id FROM workers WHERE name = ${WORKER_NAME} LIMIT 1`;
  if (existing.length) {
    const id = existing[0].id as string;
    await sql`
      INSERT INTO worker_heartbeats (worker_id, last_seen_at)
      VALUES (${id}, NOW())
      ON CONFLICT (worker_id) DO UPDATE SET last_seen_at = NOW(), updated_at = NOW()`;
    await sql`UPDATE workers SET status = 'active', updated_at = NOW() WHERE id = ${id}`;
    return id;
  }
  const [w] = await sql`
    INSERT INTO workers (name, status) VALUES (${WORKER_NAME}, 'active') RETURNING id`;
  await sql`INSERT INTO worker_heartbeats (worker_id, last_seen_at) VALUES (${w.id}, NOW())`;
  return w.id as string;
}

async function reaper(sql: ReturnType<typeof getSql>) {
  await sql`
    UPDATE jobs SET status = 'queued', worker_id = NULL, claimed_at = NULL,
      lease_expires_at = NULL, updated_at = NOW()
    WHERE status IN ('claimed', 'running') AND lease_expires_at < NOW()`;
}

async function executeJob(
  sql: ReturnType<typeof getSql>,
  job: Record<string, unknown>,
  workerId: string
) {
  const jobId = job.id as string;
  const attempt = Number(job.attempts ?? 0) + 1;
  const payload =
    typeof job.payload === "string" ? JSON.parse(job.payload) : (job.payload as Record<string, unknown>);

  await sql`UPDATE jobs SET status = 'running', attempts = ${attempt}, updated_at = NOW() WHERE id = ${jobId}`;
  const [exec] = await sql`
    INSERT INTO job_executions (job_id, worker_id, attempt, status, started_at)
    VALUES (${jobId}, ${workerId}, ${attempt}, 'running', NOW()) RETURNING id`;
  await sql`INSERT INTO job_logs (job_id, level, message) VALUES (${jobId}, 'info', 'Job started')`;

  const start = Date.now();
  let success = true;
  let errMsg = "";

  try {
    if (payload.type === "sleep") {
      const ms = Math.min(Number(payload.duration_ms) || 100, 5000);
      await new Promise((r) => setTimeout(r, ms));
    } else if (payload.type === "always_fail") {
      success = false;
      errMsg = String(payload.message || "simulated failure");
    }
  } catch (e) {
    success = false;
    errMsg = e instanceof Error ? e.message : "execution error";
  }

  const duration = Date.now() - start;

  if (success) {
    await sql`
      UPDATE jobs SET status = 'completed', completed_at = NOW(), worker_id = NULL,
        lease_expires_at = NULL, error_message = NULL, updated_at = NOW()
      WHERE id = ${jobId}`;
    await sql`
      UPDATE job_executions SET status = 'completed', finished_at = NOW(), duration_ms = ${duration}
      WHERE id = ${exec.id}`;
    await sql`INSERT INTO job_logs (job_id, level, message) VALUES (${jobId}, 'info', 'Job completed')`;
  } else {
    const maxAttempts = Number(job.max_attempts ?? 3);
    if (attempt >= maxAttempts) {
      await sql`
        UPDATE jobs SET status = 'dead_letter', failed_at = NOW(), error_message = ${errMsg},
          worker_id = NULL, lease_expires_at = NULL, updated_at = NOW()
        WHERE id = ${jobId}`;
      await sql`
        INSERT INTO dead_letter_queue (job_id, original_queue_id, payload, reason, attempts_made)
        VALUES (${jobId}, ${job.queue_id}, ${JSON.stringify(payload)}::jsonb, ${errMsg}, ${attempt})
        ON CONFLICT (job_id) DO NOTHING`;
    } else {
      await sql`
        UPDATE jobs SET status = 'queued', failed_at = NOW(), error_message = ${errMsg},
          worker_id = NULL, claimed_at = NULL, lease_expires_at = NULL,
          next_run_at = NOW() + interval '5 seconds', updated_at = NOW()
        WHERE id = ${jobId}`;
    }
    await sql`
      UPDATE job_executions SET status = 'failed', finished_at = NOW(), duration_ms = ${duration}, error = ${errMsg}
      WHERE id = ${exec.id}`;
    await sql`INSERT INTO job_logs (job_id, level, message) VALUES (${jobId}, 'error', ${errMsg})`;
  }
}

export async function runWorkerTick() {
  const sql = getSql();
  await reaper(sql);
  const workerId = await ensureWorker(sql);

  const queues = await sql`SELECT id FROM queues WHERE paused = FALSE`;
  let processed = 0;

  for (const q of queues) {
    const queueId = q.id as string;
    const budgetRows = await sql`
      SELECT GREATEST(COALESCE(concurrency_limit, 5) -
        (SELECT COUNT(*)::int FROM jobs WHERE queue_id = ${queueId} AND status IN ('claimed','running')), 0) AS budget
      FROM queues WHERE id = ${queueId}`;
    const budget = Number(budgetRows[0]?.budget ?? 0);
    if (budget <= 0) continue;

    const claimed = await sql`
      UPDATE jobs SET status = 'claimed', worker_id = ${workerId},
        claimed_at = NOW(), lease_expires_at = NOW() + (${LEASE_SECONDS} * interval '1 second'),
        updated_at = NOW()
      WHERE id IN (
        SELECT id FROM jobs
        WHERE queue_id = ${queueId} AND status = 'queued'
          AND (next_run_at IS NULL OR next_run_at <= NOW())
        ORDER BY priority DESC, next_run_at ASC NULLS FIRST
        FOR UPDATE SKIP LOCKED
        LIMIT ${budget}
      )
      RETURNING *`;

    for (const job of claimed) {
      await executeJob(sql, job as Record<string, unknown>, workerId);
      processed++;
    }
  }

  return { processed, worker: WORKER_NAME };
}
