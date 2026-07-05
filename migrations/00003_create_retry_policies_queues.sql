-- +goose Up

-- ── Retry policies ──────────────────────────────────────────────────────────
-- Standalone reference table. Not scoped to an org/project so policies can be
-- shared across queues. Deletion is restricted while any queue references it.

CREATE TABLE retry_policies (
    id              UUID            PRIMARY KEY DEFAULT gen_random_uuid(),
    name            TEXT            NOT NULL UNIQUE,
    strategy        retry_strategy  NOT NULL DEFAULT 'exponential',
    base_interval   INTERVAL        NOT NULL DEFAULT '5 seconds',
    max_interval    INTERVAL        NOT NULL DEFAULT '1 hour',
    max_attempts    INT             NOT NULL DEFAULT 5,
    created_at      TIMESTAMPTZ     NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ     NOT NULL DEFAULT NOW()
);

CREATE TRIGGER trg_retry_policies_updated_at
    BEFORE UPDATE ON retry_policies
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ── Queues ──────────────────────────────────────────────────────────────────
-- Deleting a project cascades to all its queues.

CREATE TABLE queues (
    id                  UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id          UUID        NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name                TEXT        NOT NULL,
    slug                TEXT        NOT NULL,
    retry_policy_id     UUID        REFERENCES retry_policies(id) ON DELETE SET NULL,
    concurrency_limit   INT,
    paused              BOOLEAN     NOT NULL DEFAULT FALSE,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    UNIQUE (project_id, slug)
);

CREATE INDEX idx_queues_project_id       ON queues (project_id);
CREATE INDEX idx_queues_retry_policy_id  ON queues (retry_policy_id);

CREATE TRIGGER trg_queues_updated_at
    BEFORE UPDATE ON queues
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- +goose Down

DROP TABLE IF EXISTS queues;
DROP TABLE IF EXISTS retry_policies;
