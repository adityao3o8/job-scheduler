package metrics

import (
	"context"
	"log/slog"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"codity.ai/scheduler/internal/domain"
)

// ── Counters ────────────────────────────────────────────────────────────────

var (
	JobsSubmitted = promauto.NewCounter(prometheus.CounterOpts{
		Name: "scheduler_jobs_submitted_total",
		Help: "Total number of jobs submitted to the scheduler.",
	})
	JobsCompleted = promauto.NewCounter(prometheus.CounterOpts{
		Name: "scheduler_jobs_completed_total",
		Help: "Total number of jobs completed successfully.",
	})
	JobsFailed = promauto.NewCounter(prometheus.CounterOpts{
		Name: "scheduler_jobs_failed_total",
		Help: "Total number of job execution failures.",
	})
	JobsDeadLettered = promauto.NewCounter(prometheus.CounterOpts{
		Name: "scheduler_jobs_dead_lettered_total",
		Help: "Total number of jobs moved to the dead-letter queue.",
	})
	JobRetries = promauto.NewCounter(prometheus.CounterOpts{
		Name: "scheduler_job_retries_total",
		Help: "Total number of job retries (requeued after failure).",
	})
)

// ── Histogram ───────────────────────────────────────────────────────────────

var JobDuration = promauto.NewHistogram(prometheus.HistogramOpts{
	Name:    "scheduler_job_duration_seconds",
	Help:    "Job execution duration in seconds.",
	Buckets: []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 120},
})

// ── DB-backed gauge collector ───────────────────────────────────────────────
// Implements prometheus.Collector so gauges are queried from Postgres on each
// Prometheus scrape, keeping them accurate without a background goroutine.

var (
	queueDepthDesc = prometheus.NewDesc(
		"scheduler_queue_depth",
		"Current number of jobs in queued status per queue.",
		[]string{"queue_id", "queue_name"}, nil,
	)
	activeWorkersDesc = prometheus.NewDesc(
		"scheduler_active_workers",
		"Number of workers in active status.",
		nil, nil,
	)
)

// DBCollector queries Postgres for gauge metrics on every Prometheus scrape.
type DBCollector struct {
	store  domain.MetricsStore
	logger *slog.Logger
}

func NewDBCollector(store domain.MetricsStore, logger *slog.Logger) *DBCollector {
	return &DBCollector{store: store, logger: logger}
}

func (c *DBCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- queueDepthDesc
	ch <- activeWorkersDesc
}

func (c *DBCollector) Collect(ch chan<- prometheus.Metric) {
	ctx := context.Background()

	depths, err := c.store.GetQueueDepths(ctx)
	if err != nil {
		c.logger.Error("prometheus collect queue depths", slog.String("error", err.Error()))
	} else {
		for _, d := range depths {
			ch <- prometheus.MustNewConstMetric(
				queueDepthDesc, prometheus.GaugeValue, d.Depth, d.QueueID, d.QueueName,
			)
		}
	}

	workers, err := c.store.CountActiveWorkers(ctx)
	if err != nil {
		c.logger.Error("prometheus collect active workers", slog.String("error", err.Error()))
	} else {
		ch <- prometheus.MustNewConstMetric(
			activeWorkersDesc, prometheus.GaugeValue, float64(workers),
		)
	}
}
