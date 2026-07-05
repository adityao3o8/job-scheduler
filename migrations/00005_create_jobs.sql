-- +goose Up

-- ── Jobs ────────────────────────────────────────────────────────────────────
-- Central table. Deleting a queue cascades to all its jobs.
-- Worker FK uses SET NULL so a deleted worker doesn't destroy job history.

CREATE TABLE jobs (
    id                  UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    queue_id            UUID        NOT NULL REFERENCES queues(id) ON DELETE CASCADE,
    status              job_status  NOT NULL DEFAULT 'queued',
    priority            INT         NOT NULL DEFAULT 0,
    payload             JSONB       NOT NULL,
    idempotency_key     TEXT,
    next_run_at         TIMESTAMPTZ,
    attempts            INT         NOT NULL DEFAULT 0,
    max_attempts        INT         NOT NULL DEFAULT 3,
    retry_policy_id     UUID        REFERENCES retry_policies(id) ON DELETE SET NULL,
    worker_id           UUID        REFERENCES workers(id) ON DELETE SET NULL,
    claimed_at          TIMESTAMPTZ,
    lease_expires_at    TIMESTAMPTZ,
    completed_at        TIMESTAMPTZ,
    failed_at           TIMESTAMPTZ,
    error_message       TEXT,
    cron_expr           TEXT,
    batch_id            UUID,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ─── CRITICAL: Claim-query index ────────────────────────────────────────────
--
-- The worker claim query is:
--
--   SELECT id FROM jobs
--   WHERE  status = 'queued'
--     AND  queue_id = $1
--     AND  (next_run_at IS NULL OR next_run_at <= NOW())
--   ORDER  BY priority DESC, next_run_at ASC NULLS FIRST
--   FOR UPDATE SKIP LOCKED
--   LIMIT  $2
--
-- Why this partial index matches:
--
--   1. WHERE status = 'queued'  ──  The partial predicate eliminates every
--      non-claimable row from the index. In steady state, completed/failed
--      jobs vastly outnumber queued ones, so the index stays small and hot
--      regardless of total table size.
--
--   2. queue_id (leading column)  ──  The claim targets a single queue.
--      Leading with queue_id gives an equality-match seek on the B-tree.
--
--   3. priority DESC  ──  Matches the ORDER BY priority DESC. Postgres walks
--      the B-tree in descending key order without a sort step.
--
--   4. next_run_at ASC NULLS FIRST  ──  After priority, ties are broken by
--      the earliest eligible run time. The range predicate
--      (next_run_at IS NULL OR next_run_at <= NOW()) turns into an index
--      range scan. NULLs sort first so immediately-runnable jobs appear
--      before future-scheduled ones within the same priority band.
--
-- Net effect: the claim query is an index-only scan + SKIP LOCKED, no seq scan
-- or sort, even at millions of total jobs.

CREATE INDEX idx_jobs_claim
    ON jobs (queue_id, priority DESC, next_run_at ASC NULLS FIRST)
    WHERE status = 'queued';

-- ─── Idempotency: unique per queue, only when key is set ────────────────────
-- Allows multiple jobs without an idempotency_key in the same queue.

CREATE UNIQUE INDEX idx_jobs_idempotency
    ON jobs (queue_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;

-- ─── Reaper index ───────────────────────────────────────────────────────────
-- The reaper finds jobs whose lease has expired:
--   WHERE status IN ('claimed','running') AND lease_expires_at < NOW()
-- This partial index covers exactly those rows.

CREATE INDEX idx_jobs_reaper
    ON jobs (lease_expires_at)
    WHERE status IN ('claimed', 'running');

-- ─── Operational indexes ────────────────────────────────────────────────────

-- Lease release on graceful shutdown: WHERE worker_id = $1 AND status = 'running'
CREATE INDEX idx_jobs_worker_id ON jobs (worker_id) WHERE worker_id IS NOT NULL;

-- Queue-level stats (count by status)
CREATE INDEX idx_jobs_queue_status ON jobs (queue_id, status);

-- Batch lookup
CREATE INDEX idx_jobs_batch_id ON jobs (batch_id) WHERE batch_id IS NOT NULL;

CREATE TRIGGER trg_jobs_updated_at
    BEFORE UPDATE ON jobs
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- +goose Down

DROP TABLE IF EXISTS jobs;
