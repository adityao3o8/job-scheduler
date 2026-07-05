-- +goose Up

-- ── Job executions ──────────────────────────────────────────────────────────
-- One row per attempt. Provides an audit trail even after a job reaches a
-- terminal state. Worker FK is SET NULL so execution history survives worker
-- deregistration.

CREATE TABLE job_executions (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id          UUID        NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    worker_id       UUID        REFERENCES workers(id) ON DELETE SET NULL,
    attempt         INT         NOT NULL,
    status          TEXT        NOT NULL,
    started_at      TIMESTAMPTZ NOT NULL,
    finished_at     TIMESTAMPTZ,
    error           TEXT,
    duration_ms     INT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_job_executions_job_id  ON job_executions (job_id, attempt);
CREATE INDEX idx_job_executions_worker  ON job_executions (worker_id) WHERE worker_id IS NOT NULL;

CREATE TRIGGER trg_job_executions_updated_at
    BEFORE UPDATE ON job_executions
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ── Job logs ────────────────────────────────────────────────────────────────
-- Append-only log lines emitted during execution. No updated_at (immutable).

CREATE TABLE job_logs (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id      UUID        NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    level       TEXT        NOT NULL DEFAULT 'info',
    message     TEXT        NOT NULL,
    metadata    JSONB,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_job_logs_job_created ON job_logs (job_id, created_at);

-- +goose Down

DROP TABLE IF EXISTS job_logs;
DROP TABLE IF EXISTS job_executions;
