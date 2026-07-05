package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type OrgRepository interface {
	Create(ctx context.Context, org *Organization) error
	GetByID(ctx context.Context, id uuid.UUID) (*Organization, error)
	GetBySlug(ctx context.Context, slug string) (*Organization, error)
	Update(ctx context.Context, org *Organization) error
	Delete(ctx context.Context, id uuid.UUID) error
}

type UserRepository interface {
	Create(ctx context.Context, user *User) error
	GetByID(ctx context.Context, id uuid.UUID) (*User, error)
	GetByEmail(ctx context.Context, email string) (*User, error)
}

type ProjectRepository interface {
	Create(ctx context.Context, p *Project) error
	GetByID(ctx context.Context, id, orgID uuid.UUID) (*Project, error)
	List(ctx context.Context, orgID uuid.UUID, params PageParams, nameFilter string) (*Page[Project], error)
	Update(ctx context.Context, p *Project) error
	Delete(ctx context.Context, id, orgID uuid.UUID) error
}

type QueueRepository interface {
	Create(ctx context.Context, q *Queue) error
	GetByID(ctx context.Context, id, orgID uuid.UUID) (*Queue, error)
	List(ctx context.Context, orgID uuid.UUID, params PageParams, filters QueueFilters) (*Page[Queue], error)
	Update(ctx context.Context, q *Queue, orgID uuid.UUID) error
	Delete(ctx context.Context, id, orgID uuid.UUID) error
	SetPaused(ctx context.Context, id, orgID uuid.UUID, paused bool) error
}

type JobRepository interface {
	Create(ctx context.Context, input *CreateJobInput) (*CreateJobResult, error)
	GetByID(ctx context.Context, id, orgID uuid.UUID) (*Job, error)
	// ClaimJobs atomically claims up to the queue's remaining concurrency
	// budget. Claimed jobs get status='claimed', the given worker_id, and a
	// lease that expires after leaseSeconds. Returns the claimed jobs (may be
	// empty if none are eligible or the budget is exhausted).
	ClaimJobs(ctx context.Context, workerID, queueID uuid.UUID, leaseSeconds int) ([]Job, error)
	// RetryDLQJob resets a dead-letter job back to queued with attempts=0 and
	// removes its dead_letter_queue row. Returns the updated job.
	RetryDLQJob(ctx context.Context, id, orgID uuid.UUID) (*Job, error)
	ListByQueue(ctx context.Context, queueID, orgID uuid.UUID, status string, params PageParams) (*Page[Job], error)
	ListAll(ctx context.Context, orgID uuid.UUID, status string, queueID *uuid.UUID, params PageParams) (*Page[Job], error)
	GetExecutions(ctx context.Context, jobID, orgID uuid.UUID) ([]JobExecution, error)
	GetLogs(ctx context.Context, jobID, orgID uuid.UUID) ([]JobLogEntry, error)
	ListDLQ(ctx context.Context, orgID uuid.UUID, params PageParams) (*Page[DLQListItem], error)
}

// WorkerStore is the single interface the worker engine depends on for all
// its database operations. Keeping it as one interface avoids the engine
// importing five separate repositories.
type WorkerStore interface {
	RegisterWorker(ctx context.Context, name string) (uuid.UUID, error)
	DeregisterWorker(ctx context.Context, workerID uuid.UUID) error
	ListActiveQueues(ctx context.Context) ([]Queue, error)
	ClaimJobs(ctx context.Context, workerID, queueID uuid.UUID, leaseSeconds int) ([]Job, error)
	SetJobRunning(ctx context.Context, jobID uuid.UUID) error
	CompleteJob(ctx context.Context, jobID uuid.UUID) error
	// FailJob records the failure. The caller decides whether to retry or DLQ.
	FailJob(ctx context.Context, jobID uuid.UUID, errMsg string, newAttempts int) error
	// RequeueForRetry sets status='queued' with a computed next_run_at for the
	// next attempt. Called when attempts < max_attempts.
	RequeueForRetry(ctx context.Context, jobID uuid.UUID, nextRunAt time.Time, attempts int) error
	// MoveToDLQ transitions a job to 'dead_letter' and inserts a row in the
	// dead_letter_queue table. Called when attempts >= max_attempts.
	MoveToDLQ(ctx context.Context, jobID uuid.UUID, reason string, attempts int) error
	// GetRetryPolicy fetches the retry policy for a given ID.
	GetRetryPolicy(ctx context.Context, policyID uuid.UUID) (*RetryPolicy, error)
	ReleaseJobs(ctx context.Context, workerID uuid.UUID) (int, error)
	IsIdempotencyKeyCompleted(ctx context.Context, key string) (bool, error)
	RecordExecution(ctx context.Context, exec *JobExecution) error
	AppendJobLog(ctx context.Context, entry *JobLogEntry) error
	Heartbeat(ctx context.Context, workerID uuid.UUID, leaseSeconds int) error
}

// MetricsStore provides read-only queries for the stats/metrics endpoints
// and the Prometheus gauge collector.
type MetricsStore interface {
	GetQueueStats(ctx context.Context, queueID uuid.UUID) (*QueueStats, error)
	ListWorkers(ctx context.Context) ([]WorkerInfo, error)
	GetQueueDepths(ctx context.Context) ([]QueueDepthRow, error)
	CountActiveWorkers(ctx context.Context) (int, error)
}

// ReaperStore is the interface the reaper process depends on.
type ReaperStore interface {
	// RequeueOrphanedJobs sets orphaned jobs (lease expired while claimed/running)
	// back to queued. Returns the IDs of reclaimed jobs.
	RequeueOrphanedJobs(ctx context.Context) ([]uuid.UUID, error)
	// MarkDeadWorkers sets workers whose last heartbeat is older than the
	// staleness threshold to 'dead'. Returns the count of workers marked.
	MarkDeadWorkers(ctx context.Context, staleThreshold time.Duration) (int, error)
}

// TxManager abstracts database transactions so handlers can compose multiple
// repository calls atomically without importing pgx.
type TxManager interface {
	WithTx(ctx context.Context, fn func(ctx context.Context) error) error
}
