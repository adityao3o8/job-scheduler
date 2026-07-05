package reaper

import (
	"context"
	"log/slog"
	"time"

	"codity.ai/scheduler/internal/domain"
)

// Config holds tunable parameters for the reaper loop.
type Config struct {
	Interval       time.Duration // how often the reaper runs
	StaleThreshold time.Duration // 3 × heartbeat interval
}

// Reaper periodically reclaims orphaned jobs whose lease has expired and marks
// workers with stale heartbeats as dead. It is safe to run as a standalone
// process or as a goroutine inside the API server for single-node dev.
type Reaper struct {
	store  domain.ReaperStore
	logger *slog.Logger
	cfg    Config
}

func New(store domain.ReaperStore, logger *slog.Logger, cfg Config) *Reaper {
	return &Reaper{store: store, logger: logger, cfg: cfg}
}

// Run blocks until ctx is cancelled, running the reaper sweep on every tick.
func (r *Reaper) Run(ctx context.Context) {
	r.logger.Info("reaper started",
		slog.Duration("interval", r.cfg.Interval),
		slog.Duration("stale_threshold", r.cfg.StaleThreshold))

	// Sweep immediately on startup, then on each tick.
	r.sweep(ctx)

	ticker := time.NewTicker(r.cfg.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			r.logger.Info("reaper stopped")
			return
		case <-ticker.C:
			r.sweep(ctx)
		}
	}
}

// RunOnce executes a single reaper sweep. Useful for testing.
func (r *Reaper) RunOnce(ctx context.Context) {
	r.sweep(ctx)
}

func (r *Reaper) sweep(ctx context.Context) {
	// 1. Requeue orphaned jobs.
	ids, err := r.store.RequeueOrphanedJobs(ctx)
	if err != nil {
		r.logger.Error("requeue orphaned jobs", slog.String("error", err.Error()))
	} else if len(ids) > 0 {
		for _, id := range ids {
			r.logger.Info("reclaimed orphaned job", slog.String("job_id", id.String()))
		}
		r.logger.Info("reaper reclaimed jobs", slog.Int("count", len(ids)))
	}

	// 2. Mark stale workers as dead.
	dead, err := r.store.MarkDeadWorkers(ctx, r.cfg.StaleThreshold)
	if err != nil {
		r.logger.Error("mark dead workers", slog.String("error", err.Error()))
	} else if dead > 0 {
		r.logger.Info("marked stale workers as dead", slog.Int("count", dead))
	}
}
