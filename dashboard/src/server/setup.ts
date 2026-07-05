import { getSql, getPool } from "./db";

const STATEMENTS = [
  `CREATE EXTENSION IF NOT EXISTS "pgcrypto"`,
  `DO $$ BEGIN
    CREATE TYPE job_status AS ENUM ('queued','scheduled','claimed','running','completed','failed','dead_letter');
  EXCEPTION WHEN duplicate_object THEN NULL; END $$`,
  `DO $$ BEGIN
    CREATE TYPE worker_status AS ENUM ('active','draining','offline');
  EXCEPTION WHEN duplicate_object THEN NULL; END $$`,
  `CREATE OR REPLACE FUNCTION set_updated_at() RETURNS TRIGGER AS $$
   BEGIN NEW.updated_at = NOW(); RETURN NEW; END; $$ LANGUAGE plpgsql`,
  `CREATE TABLE IF NOT EXISTS organizations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL, slug TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW())`,
  `CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    email TEXT NOT NULL, name TEXT NOT NULL, role TEXT NOT NULL DEFAULT 'member',
    password_hash TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (org_id, email))`,
  `CREATE TABLE IF NOT EXISTS projects (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name TEXT NOT NULL, slug TEXT NOT NULL, description TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (org_id, slug))`,
  `CREATE TABLE IF NOT EXISTS queues (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name TEXT NOT NULL, slug TEXT NOT NULL,
    retry_policy_id UUID, priority_default INT NOT NULL DEFAULT 0,
    concurrency_limit INT, paused BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (project_id, slug))`,
  `CREATE TABLE IF NOT EXISTS workers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL, status worker_status NOT NULL DEFAULT 'active',
    meta JSONB, created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW())`,
  `CREATE TABLE IF NOT EXISTS worker_heartbeats (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    worker_id UUID NOT NULL UNIQUE REFERENCES workers(id) ON DELETE CASCADE,
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    metadata JSONB, created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW())`,
  `CREATE TABLE IF NOT EXISTS jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    queue_id UUID NOT NULL REFERENCES queues(id) ON DELETE CASCADE,
    status job_status NOT NULL DEFAULT 'queued',
    priority INT NOT NULL DEFAULT 0, payload JSONB NOT NULL,
    idempotency_key TEXT, next_run_at TIMESTAMPTZ DEFAULT NOW(),
    attempts INT NOT NULL DEFAULT 0, max_attempts INT NOT NULL DEFAULT 3,
    retry_policy_id UUID, worker_id UUID REFERENCES workers(id) ON DELETE SET NULL,
    claimed_at TIMESTAMPTZ, lease_expires_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ, failed_at TIMESTAMPTZ, error_message TEXT,
    cron_expr TEXT, batch_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW())`,
  `CREATE TABLE IF NOT EXISTS job_executions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id UUID NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    worker_id UUID REFERENCES workers(id) ON DELETE SET NULL,
    attempt INT NOT NULL, status TEXT NOT NULL,
    started_at TIMESTAMPTZ NOT NULL, finished_at TIMESTAMPTZ,
    error TEXT, duration_ms INT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW())`,
  `CREATE TABLE IF NOT EXISTS job_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id UUID NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    level TEXT NOT NULL DEFAULT 'info', message TEXT NOT NULL,
    metadata JSONB, created_at TIMESTAMPTZ NOT NULL DEFAULT NOW())`,
  `CREATE TABLE IF NOT EXISTS dead_letter_queue (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id UUID NOT NULL UNIQUE REFERENCES jobs(id) ON DELETE CASCADE,
    original_queue_id UUID NOT NULL REFERENCES queues(id) ON DELETE CASCADE,
    payload JSONB NOT NULL, failed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    reason TEXT, attempts_made INT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW())`,
  `CREATE INDEX IF NOT EXISTS idx_jobs_claim ON jobs (queue_id, priority DESC, next_run_at ASC NULLS FIRST) WHERE status = 'queued'`,
  `CREATE UNIQUE INDEX IF NOT EXISTS idx_jobs_idempotency ON jobs (queue_id, idempotency_key) WHERE idempotency_key IS NOT NULL`,
  `CREATE INDEX IF NOT EXISTS idx_jobs_queue_status ON jobs (queue_id, status)`,
  `CREATE INDEX IF NOT EXISTS idx_job_executions_job_id ON job_executions (job_id, attempt)`,
  `CREATE INDEX IF NOT EXISTS idx_job_logs_job_created ON job_logs (job_id, created_at)`,
  `CREATE INDEX IF NOT EXISTS idx_job_executions_finished ON job_executions (finished_at DESC) WHERE finished_at IS NOT NULL`,
];

export async function ensureSchema() {
  const pool = getPool();
  try {
    for (const stmt of STATEMENTS) {
      await pool.query(stmt);
    }
  } finally {
    await pool.end();
  }
}

export async function seedDemo() {
  const sql = getSql();
  const existing = await sql`SELECT id FROM organizations WHERE slug = 'demo' LIMIT 1`;
  if (existing.length) return { seeded: false };

  const [org] = await sql`
    INSERT INTO organizations (name, slug) VALUES ('Demo Corp', 'demo') RETURNING id`;
  const [user] = await sql`
    INSERT INTO users (org_id, email, name, role, password_hash)
    VALUES (${org.id}, 'admin@demo.com', 'Demo Admin', 'admin', 'demo')
    RETURNING id, org_id, email, role`;
  const [project] = await sql`
    INSERT INTO projects (org_id, name, slug, description)
    VALUES (${org.id}, 'Platform', 'platform', 'Demo project')
    RETURNING id`;

  const queues = [
    { name: "Emails", slug: "emails", limit: 10 },
    { name: "Webhooks", slug: "webhooks", limit: 5 },
    { name: "Reports", slug: "reports", limit: 3 },
  ];

  for (const q of queues) {
    const [queue] = await sql`
      INSERT INTO queues (project_id, name, slug, concurrency_limit, priority_default)
      VALUES (${project.id}, ${q.name}, ${q.slug}, ${q.limit}, 5)
      RETURNING id`;
    for (let i = 0; i < 3; i++) {
      await sql`
        INSERT INTO jobs (queue_id, status, priority, payload, next_run_at)
        VALUES (${queue.id}, 'queued', 5, ${JSON.stringify({ type: "sleep", duration_ms: 200 + i * 100 })}::jsonb, NOW())`;
    }
  }

  await sql`
    INSERT INTO workers (name, status) VALUES ('vercel-worker-1', 'active')`;

  return { seeded: true, user };
}

export async function resolveDemoUser(email: string) {
  const sql = getSql();
  const rows = await sql`SELECT id, org_id, email, name, role FROM users WHERE email = ${email} LIMIT 1`;
  if (rows.length) return rows[0];
  const fallback = await sql`SELECT id, org_id, email, name, role FROM users WHERE email = 'admin@demo.com' LIMIT 1`;
  return fallback[0] ?? null;
}
