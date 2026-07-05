package store

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"codity.ai/scheduler/internal/domain"
)

// MetricsStore implements domain.MetricsStore over pgxpool.
type MetricsStore struct {
	pool *pgxpool.Pool
}

func NewMetricsStore(pool *pgxpool.Pool) *MetricsStore {
	return &MetricsStore{pool: pool}
}

// GetQueueStats computes depth, throughput, success rate, and latency
// percentiles for a single queue. Throughput is time-bucketed over the last
// 24h of job_executions (uses idx_job_executions_finished) so we never load
// all rows.
func (s *MetricsStore) GetQueueStats(ctx context.Context, queueID uuid.UUID) (*domain.QueueStats, error) {
	const q = `
		WITH depth AS (
			SELECT COUNT(*) AS cnt
			FROM   jobs
			WHERE  queue_id = $1 AND status = 'queued'
		),
		stats AS (
			SELECT
				COUNT(*) FILTER (WHERE je.status = 'completed'
					AND je.finished_at >= NOW() - interval '1 hour')  AS throughput_1h,
				COUNT(*) FILTER (WHERE je.status = 'completed')       AS throughput_24h,
				COUNT(*) FILTER (WHERE je.status = 'completed')       AS completed,
				COUNT(*) FILTER (WHERE je.status = 'failed')          AS failed,
				COALESCE(AVG(je.duration_ms)
					FILTER (WHERE je.status = 'completed'), 0)        AS avg_ms,
				COALESCE(
					PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY je.duration_ms)
					FILTER (WHERE je.status = 'completed'), 0)        AS p95_ms
			FROM job_executions je
			JOIN jobs j ON j.id = je.job_id
			WHERE j.queue_id = $1
			  AND je.finished_at >= NOW() - interval '24 hours'
		)
		SELECT d.cnt,
		       s.throughput_1h, s.throughput_24h,
		       s.completed, s.failed,
		       s.avg_ms, s.p95_ms
		FROM depth d, stats s`

	var (
		depth, tp1h, tp24h, completed, failed int
		avgMs, p95Ms                          float64
	)
	err := s.pool.QueryRow(ctx, q, queueID).Scan(
		&depth, &tp1h, &tp24h, &completed, &failed, &avgMs, &p95Ms,
	)
	if err != nil {
		return nil, fmt.Errorf("get queue stats: %w", err)
	}

	var successRate float64
	if total := completed + failed; total > 0 {
		successRate = float64(completed) / float64(total)
	}

	// Fetch queue name.
	var name string
	err = s.pool.QueryRow(ctx, `SELECT name FROM queues WHERE id = $1`, queueID).Scan(&name)
	if err != nil {
		return nil, fmt.Errorf("get queue name: %w", err)
	}

	return &domain.QueueStats{
		QueueID:       queueID,
		QueueName:     name,
		Depth:         depth,
		Throughput1h:  tp1h,
		Throughput24h: tp24h,
		SuccessRate:   successRate,
		AvgLatencyMs:  avgMs,
		P95LatencyMs:  p95Ms,
	}, nil
}

// ListWorkers returns all workers with their status, heartbeat, and in-flight
// job count. Uses idx_jobs_worker_id for the correlated subquery.
func (s *MetricsStore) ListWorkers(ctx context.Context) ([]domain.WorkerInfo, error) {
	const q = `
		SELECT w.id, w.name, w.status::text, wh.last_seen_at,
		       w.created_at, w.updated_at,
		       COALESCE((SELECT COUNT(*) FROM jobs
		                 WHERE worker_id = w.id
		                   AND status IN ('claimed', 'running')), 0) AS in_flight
		FROM workers w
		LEFT JOIN worker_heartbeats wh ON wh.worker_id = w.id
		ORDER BY w.created_at DESC`

	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list workers: %w", err)
	}
	defer rows.Close()

	var workers []domain.WorkerInfo
	for rows.Next() {
		var w domain.WorkerInfo
		if err := rows.Scan(&w.ID, &w.Name, &w.Status, &w.LastSeenAt,
			&w.CreatedAt, &w.UpdatedAt, &w.JobsInFlight); err != nil {
			return nil, fmt.Errorf("scan worker: %w", err)
		}
		workers = append(workers, w)
	}
	return workers, rows.Err()
}

// GetQueueDepths returns the current queued-job count for every queue.
// Uses idx_jobs_queue_status for the grouped count.
func (s *MetricsStore) GetQueueDepths(ctx context.Context) ([]domain.QueueDepthRow, error) {
	const q = `
		SELECT q.id::text, q.name,
		       COUNT(j.id)::float8 AS depth
		FROM   queues q
		LEFT JOIN jobs j ON j.queue_id = q.id AND j.status = 'queued'
		GROUP BY q.id, q.name`

	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("queue depths: %w", err)
	}
	defer rows.Close()

	var out []domain.QueueDepthRow
	for rows.Next() {
		var r domain.QueueDepthRow
		if err := rows.Scan(&r.QueueID, &r.QueueName, &r.Depth); err != nil {
			return nil, fmt.Errorf("scan depth: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// CountActiveWorkers returns the number of workers in 'active' status.
// Uses idx_workers_status.
func (s *MetricsStore) CountActiveWorkers(ctx context.Context) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM workers WHERE status = 'active'`,
	).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count active workers: %w", err)
	}
	return n, nil
}
