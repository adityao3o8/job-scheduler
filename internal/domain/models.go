package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type Organization struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type User struct {
	ID           uuid.UUID `json:"id"`
	OrgID        uuid.UUID `json:"org_id"`
	Email        string    `json:"email"`
	Name         string    `json:"name"`
	PasswordHash string    `json:"-"`
	Role         string    `json:"role"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Project struct {
	ID          uuid.UUID `json:"id"`
	OrgID       uuid.UUID `json:"org_id"`
	Name        string    `json:"name"`
	Slug        string    `json:"slug"`
	Description *string   `json:"description,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Queue struct {
	ID               uuid.UUID  `json:"id"`
	ProjectID        uuid.UUID  `json:"project_id"`
	Name             string     `json:"name"`
	Slug             string     `json:"slug"`
	RetryPolicyID    *uuid.UUID `json:"retry_policy_id,omitempty"`
	PriorityDefault  int        `json:"priority_default"`
	ConcurrencyLimit *int       `json:"concurrency_limit,omitempty"`
	IsPaused         bool       `json:"is_paused"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

// PageParams holds cursor-based pagination parameters.
type PageParams struct {
	Limit  int
	Cursor string
}

// Page is a generic paginated result.
type Page[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"next_cursor,omitempty"`
	HasMore    bool   `json:"has_more"`
}

type QueueFilters struct {
	Name      string
	IsPaused  *bool
	ProjectID *uuid.UUID
}

// ─── Jobs ───────────────────────────────────────────────────────────────────

type Job struct {
	ID             uuid.UUID       `json:"id"`
	QueueID        uuid.UUID       `json:"queue_id"`
	Status         string          `json:"status"`
	Priority       int             `json:"priority"`
	Payload        json.RawMessage `json:"payload"`
	IdempotencyKey *string         `json:"idempotency_key,omitempty"`
	NextRunAt      *time.Time      `json:"next_run_at,omitempty"`
	Attempts       int             `json:"attempts"`
	MaxAttempts    int             `json:"max_attempts"`
	RetryPolicyID  *uuid.UUID      `json:"retry_policy_id,omitempty"`
	WorkerID       *uuid.UUID      `json:"worker_id,omitempty"`
	ClaimedAt      *time.Time      `json:"claimed_at,omitempty"`
	LeaseExpiresAt *time.Time      `json:"lease_expires_at,omitempty"`
	CompletedAt    *time.Time      `json:"completed_at,omitempty"`
	FailedAt       *time.Time      `json:"failed_at,omitempty"`
	ErrorMessage   *string         `json:"error_message,omitempty"`
	CronExpr       *string         `json:"cron_expr,omitempty"`
	BatchID        *uuid.UUID      `json:"batch_id,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

type CreateJobInput struct {
	QueueID        uuid.UUID
	Priority       int
	Payload        json.RawMessage
	IdempotencyKey *string
	NextRunAt      *time.Time
	Status         string
	MaxAttempts    int
	CronExpr       *string
	BatchID        *uuid.UUID
}

type CreateJobResult struct {
	Job     *Job
	Created bool // false = idempotency hit, returned existing row
}

// ─── Retry ───────────────────────────────────────────────────────────────────

type RetryPolicy struct {
	ID           uuid.UUID     `json:"id"`
	Name         string        `json:"name"`
	Strategy     string        `json:"strategy"` // fixed | linear | exponential
	BaseInterval time.Duration `json:"base_interval"`
	MaxInterval  time.Duration `json:"max_interval"`
	MaxAttempts  int           `json:"max_attempts"`
	CreatedAt    time.Time     `json:"created_at"`
	UpdatedAt    time.Time     `json:"updated_at"`
}

type DeadLetterEntry struct {
	ID              uuid.UUID       `json:"id"`
	JobID           uuid.UUID       `json:"job_id"`
	OriginalQueueID uuid.UUID       `json:"original_queue_id"`
	Payload         json.RawMessage `json:"payload"`
	FailedAt        time.Time       `json:"failed_at"`
	Reason          *string         `json:"reason,omitempty"`
	AttemptsMade    int             `json:"attempts_made"`
	CreatedAt       time.Time       `json:"created_at"`
}

type DLQListItem struct {
	ID              uuid.UUID       `json:"id"`
	JobID           uuid.UUID       `json:"job_id"`
	OriginalQueueID uuid.UUID       `json:"original_queue_id"`
	QueueName       string          `json:"queue_name"`
	Payload         json.RawMessage `json:"payload"`
	FailedAt        time.Time       `json:"failed_at"`
	Reason          *string         `json:"reason,omitempty"`
	AttemptsMade    int             `json:"attempts_made"`
	CreatedAt       time.Time       `json:"created_at"`
}

// ─── Worker types ───────────────────────────────────────────────────────────

type JobExecution struct {
	ID         uuid.UUID  `json:"id"`
	JobID      uuid.UUID  `json:"job_id"`
	WorkerID   uuid.UUID  `json:"worker_id"`
	Attempt    int        `json:"attempt"`
	Status     string     `json:"status"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	Error      *string    `json:"error,omitempty"`
	DurationMs *int       `json:"duration_ms,omitempty"`
}

type JobLogEntry struct {
	ID        uuid.UUID       `json:"id,omitempty"`
	JobID     uuid.UUID       `json:"job_id"`
	Level     string          `json:"level"`
	Message   string          `json:"message"`
	Metadata  json.RawMessage `json:"metadata,omitempty"`
	CreatedAt *time.Time      `json:"created_at,omitempty"`
}

// ─── Metrics / Stats ────────────────────────────────────────────────────────

type QueueStats struct {
	QueueID       uuid.UUID `json:"queue_id"`
	QueueName     string    `json:"queue_name"`
	Depth         int       `json:"depth"`
	Throughput1h  int       `json:"throughput_1h"`
	Throughput24h int       `json:"throughput_24h"`
	SuccessRate   float64   `json:"success_rate"`
	AvgLatencyMs  float64   `json:"avg_latency_ms"`
	P95LatencyMs  float64   `json:"p95_latency_ms"`
}

type WorkerInfo struct {
	ID           uuid.UUID  `json:"id"`
	Name         string     `json:"name"`
	Status       string     `json:"status"`
	LastSeenAt   *time.Time `json:"last_seen_at,omitempty"`
	JobsInFlight int        `json:"jobs_in_flight"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

type QueueDepthRow struct {
	QueueID   string
	QueueName string
	Depth     float64
}

// Claims represents the authenticated user's identity extracted from a JWT.
type Claims struct {
	UserID uuid.UUID `json:"user_id"`
	OrgID  uuid.UUID `json:"org_id"`
	Email  string    `json:"email"`
	Role   string    `json:"role"`
}
