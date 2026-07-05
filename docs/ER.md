# Entity-Relationship Diagram

```mermaid
erDiagram
    organizations {
        UUID id PK
        TEXT name
        TEXT slug UK
        TIMESTAMPTZ created_at
        TIMESTAMPTZ updated_at
    }

    users {
        UUID id PK
        UUID org_id FK
        TEXT email
        TEXT name
        TEXT role
        TIMESTAMPTZ created_at
        TIMESTAMPTZ updated_at
    }

    projects {
        UUID id PK
        UUID org_id FK
        TEXT name
        TEXT slug
        TEXT description
        TIMESTAMPTZ created_at
        TIMESTAMPTZ updated_at
    }

    retry_policies {
        UUID id PK
        TEXT name UK
        retry_strategy strategy
        INTERVAL base_interval
        INTERVAL max_interval
        INT max_attempts
        TIMESTAMPTZ created_at
        TIMESTAMPTZ updated_at
    }

    queues {
        UUID id PK
        UUID project_id FK
        TEXT name
        TEXT slug
        UUID retry_policy_id FK
        INT concurrency_limit
        BOOLEAN paused
        TIMESTAMPTZ created_at
        TIMESTAMPTZ updated_at
    }

    workers {
        UUID id PK
        TEXT name
        worker_status status
        JSONB meta
        TIMESTAMPTZ created_at
        TIMESTAMPTZ updated_at
    }

    worker_heartbeats {
        UUID id PK
        UUID worker_id FK,UK
        TIMESTAMPTZ last_seen_at
        JSONB metadata
        TIMESTAMPTZ created_at
        TIMESTAMPTZ updated_at
    }

    jobs {
        UUID id PK
        UUID queue_id FK
        job_status status
        INT priority
        JSONB payload
        TEXT idempotency_key
        TIMESTAMPTZ next_run_at
        INT attempts
        INT max_attempts
        UUID retry_policy_id FK
        UUID worker_id FK
        TIMESTAMPTZ claimed_at
        TIMESTAMPTZ lease_expires_at
        TIMESTAMPTZ completed_at
        TIMESTAMPTZ failed_at
        TEXT error_message
        TEXT cron_expr
        UUID batch_id
        TIMESTAMPTZ created_at
        TIMESTAMPTZ updated_at
    }

    job_executions {
        UUID id PK
        UUID job_id FK
        UUID worker_id FK
        INT attempt
        TEXT status
        TIMESTAMPTZ started_at
        TIMESTAMPTZ finished_at
        TEXT error
        INT duration_ms
        TIMESTAMPTZ created_at
        TIMESTAMPTZ updated_at
    }

    job_logs {
        UUID id PK
        UUID job_id FK
        TEXT level
        TEXT message
        JSONB metadata
        TIMESTAMPTZ created_at
    }

    scheduled_jobs {
        UUID id PK
        UUID queue_id FK
        TEXT name
        TEXT cron_expr
        JSONB payload
        TIMESTAMPTZ next_run_at
        TIMESTAMPTZ last_run_at
        BOOLEAN enabled
        INT max_attempts
        UUID retry_policy_id FK
        TIMESTAMPTZ created_at
        TIMESTAMPTZ updated_at
    }

    dead_letter_queue {
        UUID id PK
        UUID job_id FK,UK
        UUID original_queue_id FK
        JSONB payload
        TIMESTAMPTZ failed_at
        TEXT reason
        INT attempts_made
        TIMESTAMPTZ created_at
    }

    %% ── Relationships ───────────────────────────────────────────────────

    organizations ||--o{ users             : "has members"
    organizations ||--o{ projects          : "owns"
    projects      ||--o{ queues            : "contains"
    queues        ||--o{ jobs              : "holds"
    queues        ||--o{ scheduled_jobs    : "schedules"
    retry_policies ||--o{ queues           : "configures"
    retry_policies ||--o{ jobs             : "governs"
    retry_policies ||--o{ scheduled_jobs   : "governs"
    workers       ||--|| worker_heartbeats : "reports"
    workers       ||--o{ jobs              : "claims"
    workers       ||--o{ job_executions    : "runs"
    jobs          ||--o{ job_executions    : "produces"
    jobs          ||--o{ job_logs          : "emits"
    jobs          ||--|| dead_letter_queue  : "archived in"
    queues        ||--o{ dead_letter_queue : "originates"
```

## Cascade Behavior

| Parent deleted | Cascades to | ON DELETE |
|---|---|---|
| `organizations` | `users`, `projects` → `queues` → `jobs` → `job_executions`, `job_logs`, `dead_letter_queue` | CASCADE (transitive) |
| `projects` | `queues` → `jobs` → … | CASCADE |
| `queues` | `jobs`, `scheduled_jobs`, `dead_letter_queue` | CASCADE |
| `jobs` | `job_executions`, `job_logs`, `dead_letter_queue` | CASCADE |
| `workers` | `worker_heartbeats` (CASCADE), `jobs.worker_id` (SET NULL), `job_executions.worker_id` (SET NULL) | Mixed |
| `retry_policies` | `queues.retry_policy_id`, `jobs.retry_policy_id`, `scheduled_jobs.retry_policy_id` | SET NULL |

## Key Indexes

| Index | Type | Purpose |
|---|---|---|
| `idx_jobs_claim` | Partial B-tree `(queue_id, priority DESC, next_run_at) WHERE status = 'queued'` | Atomic claim query — index-only scan, no sort |
| `idx_jobs_idempotency` | Unique partial `(queue_id, idempotency_key) WHERE idempotency_key IS NOT NULL` | Deduplicate within a queue |
| `idx_jobs_reaper` | Partial B-tree `(lease_expires_at) WHERE status IN ('claimed','running')` | Reaper stale-lease scan |
| `idx_jobs_worker_id` | Partial B-tree `(worker_id) WHERE worker_id IS NOT NULL` | Lease release on graceful shutdown |
| `idx_worker_heartbeats_last_seen` | B-tree `(last_seen_at)` | Stale worker detection |
| `idx_scheduled_jobs_due` | Partial B-tree `(next_run_at) WHERE enabled` | Cron tick evaluation |
