-- +goose Up

-- ── Workers ─────────────────────────────────────────────────────────────────
-- Global — not scoped to org/project. Deletion does NOT cascade to jobs;
-- jobs.worker_id is SET NULL on worker deletion.

CREATE TABLE workers (
    id          UUID            PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT            NOT NULL,
    status      worker_status   NOT NULL DEFAULT 'active',
    meta        JSONB,
    created_at  TIMESTAMPTZ     NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ     NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_workers_status ON workers (status);

CREATE TRIGGER trg_workers_updated_at
    BEFORE UPDATE ON workers
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ── Worker heartbeats ───────────────────────────────────────────────────────
-- One row per worker, upserted on every heartbeat. The reaper uses the index
-- on last_seen_at to find stale workers efficiently.

CREATE TABLE worker_heartbeats (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    worker_id       UUID        NOT NULL UNIQUE REFERENCES workers(id) ON DELETE CASCADE,
    last_seen_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    metadata        JSONB,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Stale-detection query: WHERE last_seen_at < NOW() - interval '60s'
CREATE INDEX idx_worker_heartbeats_last_seen ON worker_heartbeats (last_seen_at);

CREATE TRIGGER trg_worker_heartbeats_updated_at
    BEFORE UPDATE ON worker_heartbeats
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- +goose Down

DROP TABLE IF EXISTS worker_heartbeats;
DROP TABLE IF EXISTS workers;
