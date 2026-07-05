package worker

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	"codity.ai/scheduler/internal/domain"
	"codity.ai/scheduler/pkg/metrics"
	"codity.ai/scheduler/pkg/retry"
)

// Config holds all tunable parameters for the worker engine.
type Config struct {
	WorkerName        string
	MaxConcurrency    int
	PollInterval      time.Duration
	LeaseSeconds      int
	HeartbeatInterval time.Duration
	DrainTimeout      time.Duration
}

// Engine is the core worker loop. It polls Postgres for claimable jobs,
// dispatches them through a bounded goroutine pool, and handles heartbeat,
// idempotency checks, execution recording, and graceful shutdown.
type Engine struct {
	store    domain.WorkerStore
	handlers map[string]JobHandler
	logger   *slog.Logger
	cfg      Config

	workerID uuid.UUID
	sem      chan struct{}
	wg       sync.WaitGroup
}

func NewEngine(store domain.WorkerStore, logger *slog.Logger, cfg Config) *Engine {
	return &Engine{
		store:    store,
		handlers: make(map[string]JobHandler),
		logger:   logger,
		cfg:      cfg,
		sem:      make(chan struct{}, cfg.MaxConcurrency),
	}
}

// RegisterHandler adds a handler for a given job type string.
// The type is read from the payload's "type" JSON field at dispatch time.
func (e *Engine) RegisterHandler(jobType string, h JobHandler) {
	e.handlers[jobType] = h
}

// Run is the worker's main lifecycle: register → heartbeat → poll → shutdown.
// It blocks until ctx is cancelled (SIGTERM/SIGINT).
func (e *Engine) Run(ctx context.Context) error {
	id, err := e.store.RegisterWorker(ctx, e.cfg.WorkerName)
	if err != nil {
		return err
	}
	e.workerID = id
	e.logger.Info("worker registered",
		slog.String("worker_id", id.String()),
		slog.String("name", e.cfg.WorkerName),
		slog.Int("max_concurrency", e.cfg.MaxConcurrency))

	go e.heartbeatLoop(ctx)
	e.pollLoop(ctx)
	e.shutdown()
	return nil
}

// ── Poll loop ───────────────────────────────────────────────────────────────

func (e *Engine) pollLoop(ctx context.Context) {
	// Poll immediately on startup, then on each tick.
	e.pollOnce(ctx)

	ticker := time.NewTicker(e.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.pollOnce(ctx)
		}
	}
}

func (e *Engine) pollOnce(ctx context.Context) {
	if len(e.sem) >= cap(e.sem) {
		return // all slots busy, skip this tick
	}

	queues, err := e.store.ListActiveQueues(ctx)
	if err != nil {
		e.logger.Error("list queues", slog.String("error", err.Error()))
		return
	}

	for _, q := range queues {
		if ctx.Err() != nil {
			return
		}
		jobs, err := e.store.ClaimJobs(ctx, e.workerID, q.ID, e.cfg.LeaseSeconds)
		if err != nil {
			e.logger.Error("claim jobs",
				slog.String("queue", q.Name),
				slog.String("error", err.Error()))
			continue
		}
		for _, job := range jobs {
			e.dispatch(ctx, job)
		}
	}
}

func (e *Engine) dispatch(ctx context.Context, job domain.Job) {
	e.sem <- struct{}{} // acquire pool slot (blocks if at capacity)
	e.wg.Add(1)
	go func() {
		defer func() {
			<-e.sem // release slot
			e.wg.Done()
		}()
		e.executeJob(ctx, job)
	}()
}

// ── Job execution ───────────────────────────────────────────────────────────

func (e *Engine) executeJob(ctx context.Context, job domain.Job) {
	log := e.logger.With(
		slog.String("job_id", job.ID.String()),
		slog.String("queue_id", job.QueueID.String()))

	// Idempotency gate: if this key already produced a completed execution,
	// short-circuit to completed without running side effects again.
	if job.IdempotencyKey != nil {
		done, err := e.store.IsIdempotencyKeyCompleted(ctx, *job.IdempotencyKey)
		if err != nil {
			log.Error("idempotency check failed", slog.String("error", err.Error()))
		}
		if done {
			log.Info("idempotency hit, skipping execution")
			if err := e.store.CompleteJob(ctx, job.ID); err != nil {
				log.Error("complete (idempotent)", slog.String("error", err.Error()))
			}
			return
		}
	}

	// Transition claimed → running.
	if err := e.store.SetJobRunning(ctx, job.ID); err != nil {
		log.Error("set running", slog.String("error", err.Error()))
		return
	}

	// Resolve handler by payload "type" field.
	jobType := extractType(job.Payload)
	handler, ok := e.handlers[jobType]
	if !ok {
		errMsg := "unknown handler type: " + jobType
		log.Error(errMsg)
		e.handleFailure(ctx, job, errMsg, time.Now(), time.Now())
		return
	}

	log.Info("executing", slog.String("type", jobType))
	_ = e.store.AppendJobLog(ctx, &domain.JobLogEntry{
		JobID: job.ID, Level: "info", Message: "execution started, type=" + jobType,
	})

	start := time.Now()
	execErr := handler.Handle(ctx, job)
	finish := time.Now()

	if execErr != nil {
		log.Error("handler failed",
			slog.String("type", jobType),
			slog.String("error", execErr.Error()))
		e.handleFailure(ctx, job, execErr.Error(), start, finish)
		return
	}

	// Success path.
	if err := e.store.CompleteJob(ctx, job.ID); err != nil {
		log.Error("complete job", slog.String("error", err.Error()))
	}
	_ = e.store.AppendJobLog(ctx, &domain.JobLogEntry{
		JobID: job.ID, Level: "info", Message: "completed successfully",
	})

	elapsed := finish.Sub(start)
	dur := int(elapsed.Milliseconds())
	_ = e.store.RecordExecution(ctx, &domain.JobExecution{
		JobID: job.ID, WorkerID: e.workerID,
		Attempt: job.Attempts + 1, Status: "completed",
		StartedAt: start, FinishedAt: &finish, DurationMs: &dur,
	})

	metrics.JobsCompleted.Inc()
	metrics.JobDuration.Observe(elapsed.Seconds())
}

// handleFailure records the execution, increments attempts, and decides:
//   - attempts < max_attempts → requeue with computed backoff delay
//   - attempts >= max_attempts → move to dead-letter queue
func (e *Engine) handleFailure(ctx context.Context, job domain.Job, errMsg string, start, finish time.Time) {
	newAttempts := job.Attempts + 1
	log := e.logger.With(
		slog.String("job_id", job.ID.String()),
		slog.Int("attempt", newAttempts),
		slog.Int("max_attempts", job.MaxAttempts))

	// Record the failed execution.
	dur := int(finish.Sub(start).Milliseconds())
	_ = e.store.RecordExecution(ctx, &domain.JobExecution{
		JobID: job.ID, WorkerID: e.workerID,
		Attempt: newAttempts, Status: "failed",
		StartedAt: start, FinishedAt: &finish, DurationMs: &dur,
		Error: &errMsg,
	})
	_ = e.store.AppendJobLog(ctx, &domain.JobLogEntry{
		JobID: job.ID, Level: "error", Message: errMsg,
	})

	metrics.JobsFailed.Inc()

	if newAttempts >= job.MaxAttempts {
		// Exhausted retries → dead-letter.
		log.Warn("max attempts reached, moving to DLQ")
		if err := e.store.MoveToDLQ(ctx, job.ID, errMsg, newAttempts); err != nil {
			log.Error("move to DLQ", slog.String("error", err.Error()))
		}
		metrics.JobsDeadLettered.Inc()
		return
	}

	// Resolve the retry policy to compute backoff.
	strategy := "exponential"
	base := 5 * time.Second
	maxDelay := time.Hour

	if job.RetryPolicyID != nil {
		rp, err := e.store.GetRetryPolicy(ctx, *job.RetryPolicyID)
		if err != nil {
			log.Error("get retry policy, using defaults", slog.String("error", err.Error()))
		} else {
			strategy = rp.Strategy
			base = rp.BaseInterval
			maxDelay = rp.MaxInterval
		}
	}

	nextRunAt := retry.NextRunAt(strategy, base, maxDelay, newAttempts, strategy == "exponential")
	log.Info("requeueing for retry",
		slog.String("strategy", strategy),
		slog.Time("next_run_at", nextRunAt))

	if err := e.store.FailJob(ctx, job.ID, errMsg, newAttempts); err != nil {
		log.Error("fail job", slog.String("error", err.Error()))
	}
	if err := e.store.RequeueForRetry(ctx, job.ID, nextRunAt, newAttempts); err != nil {
		log.Error("requeue for retry", slog.String("error", err.Error()))
	}
	metrics.JobRetries.Inc()
}

// ── Heartbeat ───────────────────────────────────────────────────────────────

func (e *Engine) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(e.cfg.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := e.store.Heartbeat(ctx, e.workerID, e.cfg.LeaseSeconds); err != nil {
				e.logger.Error("heartbeat", slog.String("error", err.Error()))
			}
		}
	}
}

// ── Graceful shutdown ───────────────────────────────────────────────────────

func (e *Engine) shutdown() {
	e.logger.Info("shutting down, draining in-flight jobs",
		slog.Duration("timeout", e.cfg.DrainTimeout))

	drainCtx, cancel := context.WithTimeout(context.Background(), e.cfg.DrainTimeout)
	defer cancel()

	done := make(chan struct{})
	go func() {
		e.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		e.logger.Info("all in-flight jobs drained")
	case <-drainCtx.Done():
		e.logger.Warn("drain timeout exceeded, proceeding with shutdown")
	}

	// Release any jobs still in 'claimed' state (never started).
	released, err := e.store.ReleaseJobs(context.Background(), e.workerID)
	if err != nil {
		e.logger.Error("release claims", slog.String("error", err.Error()))
	} else if released > 0 {
		e.logger.Info("released unclaimed jobs", slog.Int("count", released))
	}

	if err := e.store.DeregisterWorker(context.Background(), e.workerID); err != nil {
		e.logger.Error("deregister", slog.String("error", err.Error()))
	} else {
		e.logger.Info("worker deregistered")
	}
}

// ── Helpers ─────────────────────────────────────────────────────────────────

func extractType(payload json.RawMessage) string {
	var p struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(payload, &p); err != nil || p.Type == "" {
		return "unknown"
	}
	return p.Type
}
