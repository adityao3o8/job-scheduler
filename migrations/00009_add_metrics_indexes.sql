-- +goose Up

-- The stats endpoint computes throughput/latency from job_executions bounded
-- by finished_at. This index lets Postgres range-scan efficiently.
CREATE INDEX idx_job_executions_finished
    ON job_executions (finished_at DESC)
    WHERE finished_at IS NOT NULL;

-- +goose Down

DROP INDEX IF EXISTS idx_job_executions_finished;
