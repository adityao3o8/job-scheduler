package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"codity.ai/scheduler/internal/domain"
)

// WorkerStore implements domain.WorkerStore over pgxpool. It composes the
// existing JobRepository for claim operations.
type WorkerStore struct {
	pool *pgxpool.Pool
	jobs *JobRepository
}

func NewWorkerStore(pool *pgxpool.Pool) *WorkerStore {
	return &WorkerStore{pool: pool, jobs: NewJobRepository(pool)}
}

func (s *WorkerStore) RegisterWorker(ctx context.Context, name string) (uuid.UUID, error) {
	id := uuid.New()
	const q = `INSERT INTO workers (id, name, status) VALUES ($1, $2, 'active')`
	if _, err := s.pool.Exec(ctx, q, id, name); err != nil {
		return uuid.Nil, fmt.Errorf("register worker: %w", err)
	}
	// Seed the heartbeat row so the first heartbeat is an upsert.
	const hb = `INSERT INTO worker_heartbeats (worker_id, last_seen_at) VALUES ($1, NOW())`
	if _, err := s.pool.Exec(ctx, hb, id); err != nil {
		return uuid.Nil, fmt.Errorf("seed heartbeat: %w", err)
	}
	return id, nil
}

func (s *WorkerStore) DeregisterWorker(ctx context.Context, workerID uuid.UUID) error {
	const q = `UPDATE workers SET status = 'offline', updated_at = NOW() WHERE id = $1`
	if _, err := s.pool.Exec(ctx, q, workerID); err != nil {
		return fmt.Errorf("deregister worker: %w", err)
	}
	return nil
}

func (s *WorkerStore) ListActiveQueues(ctx context.Context) ([]domain.Queue, error) {
	const q = `
		SELECT id, project_id, name, slug, retry_policy_id,
			priority_default, concurrency_limit, paused, created_at, updated_at
		FROM queues WHERE NOT paused`

	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list active queues: %w", err)
	}
	defer rows.Close()

	var queues []domain.Queue
	for rows.Next() {
		var qu domain.Queue
		if err := rows.Scan(&qu.ID, &qu.ProjectID, &qu.Name, &qu.Slug,
			&qu.RetryPolicyID, &qu.PriorityDefault, &qu.ConcurrencyLimit,
			&qu.IsPaused, &qu.CreatedAt, &qu.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan queue: %w", err)
		}
		queues = append(queues, qu)
	}
	return queues, nil
}

func (s *WorkerStore) ClaimJobs(ctx context.Context, workerID, queueID uuid.UUID, leaseSeconds int) ([]domain.Job, error) {
	return s.jobs.ClaimJobs(ctx, workerID, queueID, leaseSeconds)
}

func (s *WorkerStore) SetJobRunning(ctx context.Context, jobID uuid.UUID) error {
	const q = `UPDATE jobs SET status = 'running', updated_at = NOW() WHERE id = $1`
	if _, err := s.pool.Exec(ctx, q, jobID); err != nil {
		return fmt.Errorf("set job running %s: %w", jobID, err)
	}
	return nil
}

func (s *WorkerStore) CompleteJob(ctx context.Context, jobID uuid.UUID) error {
	const q = `UPDATE jobs SET status = 'completed', completed_at = NOW(), updated_at = NOW() WHERE id = $1`
	if _, err := s.pool.Exec(ctx, q, jobID); err != nil {
		return fmt.Errorf("complete job %s: %w", jobID, err)
	}
	return nil
}

func (s *WorkerStore) FailJob(ctx context.Context, jobID uuid.UUID, errMsg string, newAttempts int) error {
	const q = `
		UPDATE jobs
		SET status = 'failed', failed_at = NOW(), error_message = $2,
		    attempts = $3, updated_at = NOW()
		WHERE id = $1`
	if _, err := s.pool.Exec(ctx, q, jobID, errMsg, newAttempts); err != nil {
		return fmt.Errorf("fail job %s: %w", jobID, err)
	}
	return nil
}

func (s *WorkerStore) RequeueForRetry(ctx context.Context, jobID uuid.UUID, nextRunAt time.Time, attempts int) error {
	const q = `
		UPDATE jobs
		SET status       = 'queued',
		    next_run_at  = $2,
		    attempts     = $3,
		    worker_id    = NULL,
		    claimed_at   = NULL,
		    lease_expires_at = NULL,
		    failed_at    = NULL,
		    error_message = NULL,
		    updated_at   = NOW()
		WHERE id = $1`
	if _, err := s.pool.Exec(ctx, q, jobID, nextRunAt, attempts); err != nil {
		return fmt.Errorf("requeue job %s: %w", jobID, err)
	}
	return nil
}

func (s *WorkerStore) MoveToDLQ(ctx context.Context, jobID uuid.UUID, reason string, attempts int) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("dlq begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	const updateQ = `
		UPDATE jobs
		SET status       = 'dead_letter',
		    attempts     = $2,
		    failed_at    = NOW(),
		    error_message = $3,
		    updated_at   = NOW()
		WHERE id = $1`
	if _, err := tx.Exec(ctx, updateQ, jobID, attempts, reason); err != nil {
		return fmt.Errorf("dlq update job: %w", err)
	}

	const insertQ = `
		INSERT INTO dead_letter_queue (job_id, original_queue_id, payload, reason, attempts_made)
		SELECT id, queue_id, payload, $2, $3 FROM jobs WHERE id = $1`
	if _, err := tx.Exec(ctx, insertQ, jobID, reason, attempts); err != nil {
		return fmt.Errorf("dlq insert: %w", err)
	}

	return tx.Commit(ctx)
}

func (s *WorkerStore) GetRetryPolicy(ctx context.Context, policyID uuid.UUID) (*domain.RetryPolicy, error) {
	const q = `
		SELECT id, name, strategy::text, base_interval, max_interval,
		       max_attempts, created_at, updated_at
		FROM retry_policies WHERE id = $1`

	var rp domain.RetryPolicy
	var baseIvl, maxIvl time.Duration
	err := s.pool.QueryRow(ctx, q, policyID).Scan(
		&rp.ID, &rp.Name, &rp.Strategy, &baseIvl, &maxIvl,
		&rp.MaxAttempts, &rp.CreatedAt, &rp.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("get retry policy: %w", err)
	}
	rp.BaseInterval = baseIvl
	rp.MaxInterval = maxIvl
	return &rp, nil
}

// ReleaseJobs returns claimed-but-not-yet-running jobs back to queued so the
// reaper or another worker can pick them up. Called during graceful shutdown.
func (s *WorkerStore) ReleaseJobs(ctx context.Context, workerID uuid.UUID) (int, error) {
	const q = `
		UPDATE jobs
		SET status = 'queued', worker_id = NULL, claimed_at = NULL,
		    lease_expires_at = NULL, updated_at = NOW()
		WHERE worker_id = $1 AND status = 'claimed'`
	tag, err := s.pool.Exec(ctx, q, workerID)
	if err != nil {
		return 0, fmt.Errorf("release jobs: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

func (s *WorkerStore) IsIdempotencyKeyCompleted(ctx context.Context, key string) (bool, error) {
	var exists bool
	const q = `SELECT EXISTS(SELECT 1 FROM jobs WHERE idempotency_key = $1 AND status = 'completed')`
	if err := s.pool.QueryRow(ctx, q, key).Scan(&exists); err != nil {
		return false, fmt.Errorf("idempotency check: %w", err)
	}
	return exists, nil
}

func (s *WorkerStore) RecordExecution(ctx context.Context, exec *domain.JobExecution) error {
	if exec.ID == uuid.Nil {
		exec.ID = uuid.New()
	}
	const q = `
		INSERT INTO job_executions (id, job_id, worker_id, attempt, status,
			started_at, finished_at, error, duration_ms)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`
	if _, err := s.pool.Exec(ctx, q,
		exec.ID, exec.JobID, exec.WorkerID, exec.Attempt, exec.Status,
		exec.StartedAt, exec.FinishedAt, exec.Error, exec.DurationMs,
	); err != nil {
		return fmt.Errorf("record execution: %w", err)
	}
	return nil
}

func (s *WorkerStore) AppendJobLog(ctx context.Context, entry *domain.JobLogEntry) error {
	const q = `INSERT INTO job_logs (job_id, level, message, metadata) VALUES ($1, $2, $3, $4)`
	if _, err := s.pool.Exec(ctx, q, entry.JobID, entry.Level, entry.Message, entry.Metadata); err != nil {
		return fmt.Errorf("append job log: %w", err)
	}
	return nil
}

// Heartbeat upserts the worker's heartbeat row and renews the lease on all
// jobs this worker currently holds (claimed or running).
func (s *WorkerStore) Heartbeat(ctx context.Context, workerID uuid.UUID, leaseSeconds int) error {
	const hb = `
		UPDATE worker_heartbeats SET last_seen_at = NOW(), updated_at = NOW()
		WHERE worker_id = $1`
	if _, err := s.pool.Exec(ctx, hb, workerID); err != nil {
		return fmt.Errorf("heartbeat upsert: %w", err)
	}

	const renew = `
		UPDATE jobs
		SET lease_expires_at = NOW() + ($2 * interval '1 second'), updated_at = NOW()
		WHERE worker_id = $1 AND status IN ('claimed', 'running')`
	if _, err := s.pool.Exec(ctx, renew, workerID, leaseSeconds); err != nil {
		return fmt.Errorf("heartbeat renew leases: %w", err)
	}
	return nil
}
