import { claimsFromRequest, demoAuthEnabled, signToken } from "./auth";
import { error, json, mapJob, mapQueue, unauthorized } from "./http";
import { getSql } from "./db";
import { resolveDemoUser } from "./setup";
import { runWorkerTickNow, scheduleWorkerTick } from "./worker-scheduler";

type Ctx = { path: string[]; req: Request };

async function requireAuth(req: Request) {
  const claims = await claimsFromRequest(req);
  if (!claims) return null;
  return claims;
}

export async function handleApi(req: Request, path: string[]): Promise<Response> {
  const method = req.method;
  const p = path.join("/");

  try {
    if (method === "POST" && p === "auth/login") {
      const body = await req.json();
      const email = String(body.email || "").trim();
      const password = String(body.password || "");
      if (!email || !password) return error("email and password required", 422);

      const sql = getSql();
      let user: Record<string, unknown> | null = null;

      if (demoAuthEnabled()) {
        user = (await resolveDemoUser(email)) as Record<string, unknown> | null;
      } else {
        const rows = await sql`SELECT id, org_id, email, name, role FROM users WHERE email = ${email} LIMIT 1`;
        user = rows[0] as Record<string, unknown> | undefined ?? null;
      }
      if (!user) return error("invalid credentials", 401, "UNAUTHORIZED");

      const token = await signToken({
        user_id: String(user.id),
        org_id: String(user.org_id),
        email: String(user.email),
        role: String(user.role),
      });
      return json({ token, user: { ...user, password_hash: undefined } });
    }

    const claims = await requireAuth(req);
    if (!claims) return unauthorized();

    const sql = getSql();
    const orgId = claims.org_id;

    if (method === "GET" && p === "projects") {
      const rows = await sql`SELECT * FROM projects WHERE org_id = ${orgId} ORDER BY created_at`;
      return json({ items: rows, has_more: false });
    }

    if (method === "POST" && p === "projects") {
      const body = await req.json();
      const [row] = await sql`
        INSERT INTO projects (org_id, name, slug, description)
        VALUES (${orgId}, ${body.name}, ${body.slug}, ${body.description ?? null})
        RETURNING *`;
      return json(row, 201);
    }

    if (method === "GET" && p === "queues") {
      const rows = await sql`
        SELECT q.* FROM queues q
        JOIN projects p ON p.id = q.project_id
        WHERE p.org_id = ${orgId}
        ORDER BY q.created_at`;
      return json({ items: rows.map((r) => mapQueue(r as Record<string, unknown>)), has_more: false });
    }

    if (method === "POST" && p === "queues") {
      const body = await req.json();
      const proj = await sql`SELECT id FROM projects WHERE id = ${body.project_id} AND org_id = ${orgId} LIMIT 1`;
      if (!proj.length) return error("project not found", 404);
      const [row] = await sql`
        INSERT INTO queues (project_id, name, slug, priority_default, concurrency_limit)
        VALUES (${body.project_id}, ${body.name}, ${body.slug},
          ${body.priority_default ?? 5}, ${body.concurrency_limit ?? null})
        RETURNING *`;
      return json(mapQueue(row as Record<string, unknown>), 201);
    }

    const queueMatch = p.match(/^queues\/([^/]+)(?:\/(.+))?$/);
    if (queueMatch) {
      const queueId = queueMatch[1];
      const sub = queueMatch[2] || "";

      if (method === "GET" && sub === "stats") {
        const [depth] = await sql`SELECT COUNT(*)::int AS cnt FROM jobs WHERE queue_id = ${queueId} AND status = 'queued'`;
        const [stats] = await sql`
          SELECT
            COUNT(*) FILTER (WHERE je.status = 'completed' AND je.finished_at >= NOW() - interval '1 hour')::int AS tp1h,
            COUNT(*) FILTER (WHERE je.status = 'completed' AND je.finished_at >= NOW() - interval '24 hours')::int AS tp24h,
            COUNT(*) FILTER (WHERE je.status = 'completed')::int AS completed,
            COUNT(*) FILTER (WHERE je.status = 'failed')::int AS failed,
            COALESCE(AVG(je.duration_ms) FILTER (WHERE je.status = 'completed'), 0) AS avg_ms
          FROM job_executions je JOIN jobs j ON j.id = je.job_id
          WHERE j.queue_id = ${queueId} AND je.finished_at >= NOW() - interval '24 hours'`;
        const [qname] = await sql`
          SELECT q.name FROM queues q JOIN projects p ON p.id = q.project_id
          WHERE q.id = ${queueId} AND p.org_id = ${orgId}`;
        if (!qname) return error("not found", 404);
        const completed = Number(stats?.completed ?? 0);
        const failed = Number(stats?.failed ?? 0);
        const total = completed + failed;
        return json({
          queue_id: queueId,
          queue_name: qname.name,
          depth: Number(depth?.cnt ?? 0),
          throughput_1h: Number(stats?.tp1h ?? 0),
          throughput_24h: Number(stats?.tp24h ?? 0),
          success_rate: total ? completed / total : 1,
          avg_latency_ms: Number(stats?.avg_ms ?? 0),
          p95_latency_ms: Number(stats?.avg_ms ?? 0),
        });
      }

      if (method === "POST" && sub === "pause") {
        await sql`
          UPDATE queues q SET paused = TRUE, updated_at = NOW()
          FROM projects p WHERE q.project_id = p.id AND q.id = ${queueId} AND p.org_id = ${orgId}`;
        return new Response(null, { status: 204 });
      }

      if (method === "POST" && sub === "resume") {
        await sql`
          UPDATE queues q SET paused = FALSE, updated_at = NOW()
          FROM projects p WHERE q.project_id = p.id AND q.id = ${queueId} AND p.org_id = ${orgId}`;
        return new Response(null, { status: 204 });
      }

      if (method === "PUT" && sub === "") {
        const body = await req.json();
        const [row] = await sql`
          UPDATE queues q SET
            name = COALESCE(${body.name ?? null}, q.name),
            priority_default = COALESCE(${body.priority_default ?? null}, q.priority_default),
            concurrency_limit = COALESCE(${body.concurrency_limit ?? null}, q.concurrency_limit),
            updated_at = NOW()
          FROM projects p WHERE q.project_id = p.id AND q.id = ${queueId} AND p.org_id = ${orgId}
          RETURNING q.*`;
        if (!row) return error("not found", 404);
        return json(mapQueue(row as Record<string, unknown>));
      }

      if (method === "POST" && sub === "jobs") {
        const body = await req.json();
        const qrows = await sql`
          SELECT q.id, q.priority_default FROM queues q JOIN projects p ON p.id = q.project_id
          WHERE q.id = ${queueId} AND p.org_id = ${orgId}`;
        if (!qrows.length) return error("not found", 404);

        if (body.idempotency_key) {
          const existing = await sql`
            SELECT * FROM jobs WHERE queue_id = ${queueId} AND idempotency_key = ${body.idempotency_key} LIMIT 1`;
          if (existing.length) return json(mapJob(existing[0] as Record<string, unknown>), 200);
        }

        const delaySec = Number(body.delay_seconds) || 0;
        const [row] =
          delaySec > 0
            ? await sql`
                INSERT INTO jobs (queue_id, status, priority, payload, idempotency_key, max_attempts, next_run_at, cron_expr)
                VALUES (
                  ${queueId}, 'queued',
                  ${body.priority ?? qrows[0].priority_default ?? 5},
                  ${JSON.stringify(body.payload)}::jsonb,
                  ${body.idempotency_key ?? null},
                  ${body.max_attempts ?? 3},
                  NOW() + (${delaySec} * interval '1 second'),
                  ${body.cron_expr ?? null}
                ) RETURNING *`
            : await sql`
                INSERT INTO jobs (queue_id, status, priority, payload, idempotency_key, max_attempts, next_run_at, cron_expr)
                VALUES (
                  ${queueId}, 'queued',
                  ${body.priority ?? qrows[0].priority_default ?? 5},
                  ${JSON.stringify(body.payload)}::jsonb,
                  ${body.idempotency_key ?? null},
                  ${body.max_attempts ?? 3},
                  NOW(),
                  ${body.cron_expr ?? null}
                ) RETURNING *`;
        scheduleWorkerTick();
        return json(mapJob(row as Record<string, unknown>), 201);
      }
    }

    if (method === "GET" && p === "worker/pulse") {
      const result = await runWorkerTickNow();
      return json({ ok: true, ...result });
    }

    if (method === "GET" && p === "jobs") {
      scheduleWorkerTick();
      const url = new URL(req.url);
      const status = url.searchParams.get("status") || "";
      const queueId = url.searchParams.get("queue_id");
      const rows = await sql`
        SELECT j.* FROM jobs j
        JOIN queues q ON q.id = j.queue_id
        JOIN projects p ON p.id = q.project_id
        WHERE p.org_id = ${orgId}
          AND (${status} = '' OR j.status::text = ${status})
          AND (${queueId ?? null}::uuid IS NULL OR j.queue_id = ${queueId ?? null}::uuid)
        ORDER BY j.created_at DESC LIMIT 100`;
      return json({ items: rows.map((r) => mapJob(r as Record<string, unknown>)), has_more: false });
    }

    const jobMatch = p.match(/^jobs\/([^/]+)(?:\/(.+))?$/);
    if (jobMatch) {
      const jobId = jobMatch[1];
      const sub = jobMatch[2] || "";

      if (method === "GET" && sub === "executions") {
        const rows = await sql`
          SELECT je.* FROM job_executions je
          JOIN jobs j ON j.id = je.job_id
          JOIN queues q ON q.id = j.queue_id
          JOIN projects p ON p.id = q.project_id
          WHERE je.job_id = ${jobId} AND p.org_id = ${orgId}
          ORDER BY je.attempt`;
        return json(rows);
      }

      if (method === "GET" && sub === "logs") {
        const rows = await sql`
          SELECT jl.* FROM job_logs jl
          JOIN jobs j ON j.id = jl.job_id
          JOIN queues q ON q.id = j.queue_id
          JOIN projects p ON p.id = q.project_id
          WHERE jl.job_id = ${jobId} AND p.org_id = ${orgId}
          ORDER BY jl.created_at`;
        return json(rows);
      }

      if (method === "POST" && sub === "retry") {
        await sql`DELETE FROM dead_letter_queue WHERE job_id = ${jobId}`;
        const [row] = await sql`
          UPDATE jobs j SET status = 'queued', attempts = 0, next_run_at = NOW(),
            failed_at = NULL, error_message = NULL, worker_id = NULL,
            claimed_at = NULL, lease_expires_at = NULL, updated_at = NOW()
          FROM queues q JOIN projects p ON p.id = q.project_id
          WHERE j.queue_id = q.id AND j.id = ${jobId} AND p.org_id = ${orgId}
          RETURNING j.*`;
        if (!row) return error("not found", 404);
        scheduleWorkerTick();
        return json(mapJob(row as Record<string, unknown>));
      }

      if (method === "GET" && sub === "") {
        const rows = await sql`
          SELECT j.* FROM jobs j
          JOIN queues q ON q.id = j.queue_id
          JOIN projects p ON p.id = q.project_id
          WHERE j.id = ${jobId} AND p.org_id = ${orgId}`;
        if (!rows.length) return error("not found", 404);
        return json(mapJob(rows[0] as Record<string, unknown>));
      }
    }

    if (method === "GET" && p === "workers") {
      const rows = await sql`
        SELECT w.id, w.name, w.status::text, wh.last_seen_at,
          w.created_at, w.updated_at,
          COALESCE((SELECT COUNT(*)::int FROM jobs WHERE worker_id = w.id AND status IN ('claimed','running')), 0) AS jobs_in_flight
        FROM workers w LEFT JOIN worker_heartbeats wh ON wh.worker_id = w.id
        ORDER BY w.created_at DESC`;
      return json(
        rows.map((w) => ({
          id: w.id,
          name: w.name,
          status: w.status,
          last_seen_at: w.last_seen_at,
          created_at: w.created_at,
          updated_at: w.updated_at,
          jobs_in_flight: w.jobs_in_flight,
        }))
      );
    }

    if (method === "GET" && p === "dlq") {
      const rows = await sql`
        SELECT d.id, d.job_id, d.original_queue_id, q.name AS queue_name,
          d.payload, d.failed_at, d.reason, d.attempts_made, d.created_at
        FROM dead_letter_queue d
        JOIN queues q ON q.id = d.original_queue_id
        JOIN projects p ON p.id = q.project_id
        WHERE p.org_id = ${orgId}
        ORDER BY d.failed_at DESC LIMIT 100`;
      return json({ items: rows, has_more: false });
    }

    return error("not found", 404, "NOT_FOUND");
  } catch (e) {
    const msg = e instanceof Error ? e.message : "internal error";
    if (msg.includes("DATABASE_URL")) return error(msg, 503, "UNAVAILABLE");
    console.error("API error:", e);
    return error(msg, 500, "INTERNAL");
  }
}
