-- +goose Up

-- ── Scheduled jobs (cron) ───────────────────────────────────────────────────
-- Defines recurring jobs. A scheduler tick evaluates next_run_at and enqueues
-- a new row into the jobs table if the schedule is due.

CREATE TABLE scheduled_jobs (
    id                  UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    queue_id            UUID        NOT NULL REFERENCES queues(id) ON DELETE CASCADE,
    name                TEXT        NOT NULL,
    cron_expr           TEXT        NOT NULL,
    payload             JSONB       NOT NULL DEFAULT '{}',
    next_run_at         TIMESTAMPTZ NOT NULL,
    last_run_at         TIMESTAMPTZ,
    enabled             BOOLEAN     NOT NULL DEFAULT TRUE,
    max_attempts        INT         NOT NULL DEFAULT 3,
    retry_policy_id     UUID        REFERENCES retry_policies(id) ON DELETE SET NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- The scheduler loop queries: WHERE enabled AND next_run_at <= NOW()
CREATE INDEX idx_scheduled_jobs_due
    ON scheduled_jobs (next_run_at)
    WHERE enabled = TRUE;

CREATE INDEX idx_scheduled_jobs_queue ON scheduled_jobs (queue_id);

CREATE TRIGGER trg_scheduled_jobs_updated_at
    BEFORE UPDATE ON scheduled_jobs
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ── Dead-letter queue ───────────────────────────────────────────────────────
-- Snapshot of a job that exhausted all retries. Kept as a separate table so
-- operators can query/inspect failures without filtering the main jobs table.
-- The 1:1 FK to jobs preserves lineage; cascading on job deletion keeps
-- referential integrity if the source job is purged.

CREATE TABLE dead_letter_queue (
    id                  UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id              UUID        NOT NULL UNIQUE REFERENCES jobs(id) ON DELETE CASCADE,
    original_queue_id   UUID        NOT NULL REFERENCES queues(id) ON DELETE CASCADE,
    payload             JSONB       NOT NULL,
    failed_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    reason              TEXT,
    attempts_made       INT         NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_dlq_queue       ON dead_letter_queue (original_queue_id, failed_at DESC);
CREATE INDEX idx_dlq_failed_at   ON dead_letter_queue (failed_at DESC);

-- +goose Down

DROP TABLE IF EXISTS dead_letter_queue;
DROP TABLE IF EXISTS scheduled_jobs;
