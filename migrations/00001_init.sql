-- +goose Up

-- gen_random_uuid() is built-in since PG 13; pgcrypto is a safety net for PG < 13.
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- ── Enum types ──────────────────────────────────────────────────────────────

CREATE TYPE job_status AS ENUM (
    'queued',
    'scheduled',
    'claimed',
    'running',
    'completed',
    'failed',
    'dead_letter'
);

CREATE TYPE retry_strategy AS ENUM (
    'fixed',
    'linear',
    'exponential'
);

CREATE TYPE worker_status AS ENUM (
    'active',
    'draining',
    'offline'
);

-- ── Reusable updated_at trigger ─────────────────────────────────────────────

CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- +goose Down

DROP FUNCTION IF EXISTS set_updated_at() CASCADE;
DROP TYPE IF EXISTS worker_status;
DROP TYPE IF EXISTS retry_strategy;
DROP TYPE IF EXISTS job_status;
DROP EXTENSION IF EXISTS "pgcrypto";
