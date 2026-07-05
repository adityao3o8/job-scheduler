package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"codity.ai/scheduler/internal/domain"
	"codity.ai/scheduler/pkg/pagination"
)

type JobRepository struct{ pool *pgxpool.Pool }

func NewJobRepository(pool *pgxpool.Pool) *JobRepository {
	return &JobRepository{pool: pool}
}

const jobCols = `j.id, j.queue_id, j.status, j.priority, j.payload,
	j.idempotency_key, j.next_run_at, j.attempts, j.max_attempts,
	j.retry_policy_id, j.worker_id, j.claimed_at, j.lease_expires_at,
	j.completed_at, j.failed_at, j.error_message, j.cron_expr, j.batch_id,
	j.created_at, j.updated_at`

// Create inserts a job. On idempotency_key conflict it returns the existing
// row with Created=false so the handler can respond 200 instead of 201.
func (r *JobRepository) Create(ctx context.Context, input *domain.CreateJobInput) (*domain.CreateJobResult, error) {
	id := uuid.New()

	const insertQ = `
		INSERT INTO jobs (id, queue_id, status, priority, payload,
			idempotency_key, next_run_at, max_attempts, cron_expr, batch_id)
		VALUES ($1, $2, $3::job_status, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (queue_id, idempotency_key) WHERE idempotency_key IS NOT NULL
		DO NOTHING
		RETURNING id, queue_id, status::text, priority, payload,
			idempotency_key, next_run_at, attempts, max_attempts,
			retry_policy_id, worker_id, claimed_at, lease_expires_at,
			completed_at, failed_at, error_message, cron_expr, batch_id,
			created_at, updated_at`

	db := conn(ctx, r.pool)
	job, err := scanJobRow(db.QueryRow(ctx, insertQ,
		id, input.QueueID, input.Status, input.Priority, input.Payload,
		input.IdempotencyKey, input.NextRunAt, input.MaxAttempts,
		input.CronExpr, input.BatchID,
	))

	if err == nil {
		return &domain.CreateJobResult{Job: job, Created: true}, nil
	}

	if !errors.Is(err, domain.ErrNotFound) {
		return nil, fmt.Errorf("insert job: %w", err)
	}

	// DO NOTHING fired — the idempotency key already exists. Fetch existing.
	if input.IdempotencyKey == nil {
		return nil, fmt.Errorf("insert job returned no rows without idempotency key: %w", err)
	}

	const selectQ = `
		SELECT id, queue_id, status::text, priority, payload,
			idempotency_key, next_run_at, attempts, max_attempts,
			retry_policy_id, worker_id, claimed_at, lease_expires_at,
			completed_at, failed_at, error_message, cron_expr, batch_id,
			created_at, updated_at
		FROM jobs
		WHERE queue_id = $1 AND idempotency_key = $2`

	existing, err := scanJobRow(db.QueryRow(ctx, selectQ, input.QueueID, *input.IdempotencyKey))
	if err != nil {
		return nil, fmt.Errorf("fetch existing job by idempotency_key: %w", err)
	}
	return &domain.CreateJobResult{Job: existing, Created: false}, nil
}

// ClaimJobs atomically claims eligible jobs up to the queue's remaining
// concurrency budget. The budget is computed under a FOR UPDATE lock on the
// queue row so concurrent claimers serialize properly.
func (r *JobRepository) ClaimJobs(ctx context.Context, workerID, queueID uuid.UUID, leaseSeconds int) ([]domain.Job, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("claim begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Lock the queue row and compute: concurrency_limit - currently_active.
	// The FOR UPDATE serializes concurrent ClaimJobs calls against the same
	// queue, preventing over-admission above the concurrency limit.
	var budget int
	err = tx.QueryRow(ctx, `
		SELECT GREATEST(
			COALESCE(q.concurrency_limit, 2147483647) -
			(SELECT COUNT(*) FROM jobs
			 WHERE queue_id = q.id AND status IN ('claimed', 'running')),
			0
		)
		FROM queues q
		WHERE q.id = $1
		FOR UPDATE`, queueID).Scan(&budget)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("queue %s: %w", queueID, domain.ErrNotFound)
		}
		return nil, fmt.Errorf("compute claim budget: %w", err)
	}
	if budget <= 0 {
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("claim commit (empty): %w", err)
		}
		return nil, nil
	}

	// Single atomic claim: the inner SELECT grabs the best rows with
	// FOR UPDATE SKIP LOCKED, the outer UPDATE transitions them in one
	// round trip. This is the correctness invariant from .cursorrules.
	const claimQ = `
		UPDATE jobs
		SET    status           = 'claimed',
		       worker_id        = $1,
		       claimed_at       = NOW(),
		       lease_expires_at = NOW() + ($2 * interval '1 second'),
		       updated_at       = NOW()
		WHERE id IN (
		    SELECT id FROM jobs
		    WHERE  queue_id = $3
		      AND  status   = 'queued'
		      AND  next_run_at <= NOW()
		    ORDER  BY priority DESC, next_run_at ASC
		    FOR UPDATE SKIP LOCKED
		    LIMIT  $4
		)
		RETURNING id, queue_id, status::text, priority, payload,
		    idempotency_key, next_run_at, attempts, max_attempts,
		    retry_policy_id, worker_id, claimed_at, lease_expires_at,
		    completed_at, failed_at, error_message, cron_expr, batch_id,
		    created_at, updated_at`

	rows, err := tx.Query(ctx, claimQ, workerID, leaseSeconds, queueID, budget)
	if err != nil {
		return nil, fmt.Errorf("claim jobs: %w", err)
	}
	defer rows.Close()

	var jobs []domain.Job
	for rows.Next() {
		var j domain.Job
		if err := rows.Scan(
			&j.ID, &j.QueueID, &j.Status, &j.Priority, &j.Payload,
			&j.IdempotencyKey, &j.NextRunAt, &j.Attempts, &j.MaxAttempts,
			&j.RetryPolicyID, &j.WorkerID, &j.ClaimedAt, &j.LeaseExpiresAt,
			&j.CompletedAt, &j.FailedAt, &j.ErrorMessage, &j.CronExpr, &j.BatchID,
			&j.CreatedAt, &j.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan claimed job: %w", err)
		}
		jobs = append(jobs, j)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("claim rows iteration: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("claim commit: %w", err)
	}
	return jobs, nil
}

// RetryDLQJob resets a dead-letter job to queued and removes its DLQ row.
func (r *JobRepository) RetryDLQJob(ctx context.Context, id, orgID uuid.UUID) (*domain.Job, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("retry dlq begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Verify job exists, belongs to org, and is in dead_letter status.
	const checkQ = `
		SELECT j.id FROM jobs j
		JOIN queues q ON q.id = j.queue_id
		JOIN projects p ON p.id = q.project_id
		WHERE j.id = $1 AND p.org_id = $2 AND j.status = 'dead_letter'`

	var foundID uuid.UUID
	err = tx.QueryRow(ctx, checkQ, id, orgID).Scan(&foundID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("retry dlq check: %w", err)
	}

	const deleteQ = `DELETE FROM dead_letter_queue WHERE job_id = $1`
	if _, err := tx.Exec(ctx, deleteQ, id); err != nil {
		return nil, fmt.Errorf("retry dlq delete: %w", err)
	}

	const updateQ = `
		UPDATE jobs
		SET status       = 'queued',
		    attempts     = 0,
		    next_run_at  = NOW(),
		    failed_at    = NULL,
		    error_message = NULL,
		    worker_id    = NULL,
		    claimed_at   = NULL,
		    lease_expires_at = NULL,
		    updated_at   = NOW()
		WHERE id = $1
		RETURNING id, queue_id, status::text, priority, payload,
			idempotency_key, next_run_at, attempts, max_attempts,
			retry_policy_id, worker_id, claimed_at, lease_expires_at,
			completed_at, failed_at, error_message, cron_expr, batch_id,
			created_at, updated_at`

	job, err := scanJobRow(tx.QueryRow(ctx, updateQ, id))
	if err != nil {
		return nil, fmt.Errorf("retry dlq update: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("retry dlq commit: %w", err)
	}
	return job, nil
}

// GetByID scopes through projects to enforce org isolation.
func (r *JobRepository) GetByID(ctx context.Context, id, orgID uuid.UUID) (*domain.Job, error) {
	q := `SELECT ` + jobCols + `
		FROM jobs j
		JOIN queues q2 ON q2.id = j.queue_id
		JOIN projects p ON p.id = q2.project_id
		WHERE j.id = $1 AND p.org_id = $2`

	row := conn(ctx, r.pool).QueryRow(ctx, q, id, orgID)
	return scanJobRow(row)
}

func scanJobRow(row pgx.Row) (*domain.Job, error) {
	var j domain.Job
	err := row.Scan(
		&j.ID, &j.QueueID, &j.Status, &j.Priority, &j.Payload,
		&j.IdempotencyKey, &j.NextRunAt, &j.Attempts, &j.MaxAttempts,
		&j.RetryPolicyID, &j.WorkerID, &j.ClaimedAt, &j.LeaseExpiresAt,
		&j.CompletedAt, &j.FailedAt, &j.ErrorMessage, &j.CronExpr, &j.BatchID,
		&j.CreatedAt, &j.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("scan job: %w", err)
	}
	return &j, nil
}

func (r *JobRepository) ListByQueue(ctx context.Context, queueID, orgID uuid.UUID, status string, params domain.PageParams) (*domain.Page[domain.Job], error) {
	limit := pagination.ClampLimit(params.Limit)

	q := `SELECT ` + jobCols + ` FROM jobs j
		JOIN queues q ON q.id = j.queue_id
		JOIN projects p ON p.id = q.project_id
		WHERE j.queue_id = $1 AND p.org_id = $2
		  AND ($3 = '' OR j.status::text = $3)
		ORDER BY j.created_at DESC
		LIMIT $4`

	rows, err := conn(ctx, r.pool).Query(ctx, q, queueID, orgID, status, limit)
	if err != nil {
		return nil, fmt.Errorf("list jobs by queue: %w", err)
	}
	defer rows.Close()

	var items []domain.Job
	for rows.Next() {
		var j domain.Job
		if err := rows.Scan(
			&j.ID, &j.QueueID, &j.Status, &j.Priority, &j.Payload,
			&j.IdempotencyKey, &j.NextRunAt, &j.Attempts, &j.MaxAttempts,
			&j.RetryPolicyID, &j.WorkerID, &j.ClaimedAt, &j.LeaseExpiresAt,
			&j.CompletedAt, &j.FailedAt, &j.ErrorMessage, &j.CronExpr, &j.BatchID,
			&j.CreatedAt, &j.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan job row: %w", err)
		}
		items = append(items, j)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list jobs by queue rows: %w", err)
	}

	return &domain.Page[domain.Job]{
		Items:   items,
		HasMore: len(items) == limit,
	}, nil
}

func (r *JobRepository) ListAll(ctx context.Context, orgID uuid.UUID, status string, queueID *uuid.UUID, params domain.PageParams) (*domain.Page[domain.Job], error) {
	limit := pagination.ClampLimit(params.Limit)

	q := `SELECT ` + jobCols + ` FROM jobs j
		JOIN queues q ON q.id = j.queue_id
		JOIN projects p ON p.id = q.project_id
		WHERE p.org_id = $1
		  AND ($2 = '' OR j.status::text = $2)
		  AND ($3::uuid IS NULL OR j.queue_id = $3)
		ORDER BY j.created_at DESC
		LIMIT $4`

	rows, err := conn(ctx, r.pool).Query(ctx, q, orgID, status, queueID, limit)
	if err != nil {
		return nil, fmt.Errorf("list all jobs: %w", err)
	}
	defer rows.Close()

	var items []domain.Job
	for rows.Next() {
		var j domain.Job
		if err := rows.Scan(
			&j.ID, &j.QueueID, &j.Status, &j.Priority, &j.Payload,
			&j.IdempotencyKey, &j.NextRunAt, &j.Attempts, &j.MaxAttempts,
			&j.RetryPolicyID, &j.WorkerID, &j.ClaimedAt, &j.LeaseExpiresAt,
			&j.CompletedAt, &j.FailedAt, &j.ErrorMessage, &j.CronExpr, &j.BatchID,
			&j.CreatedAt, &j.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan job row: %w", err)
		}
		items = append(items, j)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list all jobs rows: %w", err)
	}

	return &domain.Page[domain.Job]{
		Items:   items,
		HasMore: len(items) == limit,
	}, nil
}

func (r *JobRepository) GetExecutions(ctx context.Context, jobID, orgID uuid.UUID) ([]domain.JobExecution, error) {
	const q = `
		SELECT je.id, je.job_id, je.worker_id, je.attempt, je.status, je.started_at,
		       je.finished_at, je.error, je.duration_ms
		FROM job_executions je
		JOIN jobs j ON j.id = je.job_id
		JOIN queues q ON q.id = j.queue_id
		JOIN projects p ON p.id = q.project_id
		WHERE je.job_id = $1 AND p.org_id = $2
		ORDER BY je.attempt ASC`

	rows, err := conn(ctx, r.pool).Query(ctx, q, jobID, orgID)
	if err != nil {
		return nil, fmt.Errorf("get executions: %w", err)
	}
	defer rows.Close()

	var items []domain.JobExecution
	for rows.Next() {
		var e domain.JobExecution
		if err := rows.Scan(
			&e.ID, &e.JobID, &e.WorkerID, &e.Attempt, &e.Status, &e.StartedAt,
			&e.FinishedAt, &e.Error, &e.DurationMs,
		); err != nil {
			return nil, fmt.Errorf("scan execution: %w", err)
		}
		items = append(items, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get executions rows: %w", err)
	}
	return items, nil
}

func (r *JobRepository) GetLogs(ctx context.Context, jobID, orgID uuid.UUID) ([]domain.JobLogEntry, error) {
	const q = `
		SELECT jl.id, jl.job_id, jl.level, jl.message, jl.metadata, jl.created_at
		FROM job_logs jl
		JOIN jobs j ON j.id = jl.job_id
		JOIN queues q ON q.id = j.queue_id
		JOIN projects p ON p.id = q.project_id
		WHERE jl.job_id = $1 AND p.org_id = $2
		ORDER BY jl.created_at ASC`

	rows, err := conn(ctx, r.pool).Query(ctx, q, jobID, orgID)
	if err != nil {
		return nil, fmt.Errorf("get logs: %w", err)
	}
	defer rows.Close()

	var items []domain.JobLogEntry
	for rows.Next() {
		var e domain.JobLogEntry
		if err := rows.Scan(
			&e.ID, &e.JobID, &e.Level, &e.Message, &e.Metadata, &e.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan log entry: %w", err)
		}
		items = append(items, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get logs rows: %w", err)
	}
	return items, nil
}

func (r *JobRepository) ListDLQ(ctx context.Context, orgID uuid.UUID, params domain.PageParams) (*domain.Page[domain.DLQListItem], error) {
	limit := pagination.ClampLimit(params.Limit)

	const q = `
		SELECT d.id, d.job_id, d.original_queue_id, q.name AS queue_name,
		       d.payload, d.failed_at, d.reason, d.attempts_made, d.created_at
		FROM dead_letter_queue d
		JOIN queues q ON q.id = d.original_queue_id
		JOIN projects p ON p.id = q.project_id
		WHERE p.org_id = $1
		ORDER BY d.failed_at DESC
		LIMIT $2`

	rows, err := conn(ctx, r.pool).Query(ctx, q, orgID, limit)
	if err != nil {
		return nil, fmt.Errorf("list dlq: %w", err)
	}
	defer rows.Close()

	var items []domain.DLQListItem
	for rows.Next() {
		var d domain.DLQListItem
		if err := rows.Scan(
			&d.ID, &d.JobID, &d.OriginalQueueID, &d.QueueName,
			&d.Payload, &d.FailedAt, &d.Reason, &d.AttemptsMade, &d.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan dlq item: %w", err)
		}
		items = append(items, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list dlq rows: %w", err)
	}

	return &domain.Page[domain.DLQListItem]{
		Items:   items,
		HasMore: len(items) == limit,
	}, nil
}
