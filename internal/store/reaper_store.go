package store

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ReaperStore implements domain.ReaperStore over pgxpool.
type ReaperStore struct {
	pool *pgxpool.Pool
}

func NewReaperStore(pool *pgxpool.Pool) *ReaperStore {
	return &ReaperStore{pool: pool}
}

// RequeueOrphanedJobs finds jobs whose lease has expired while still in
// claimed/running status and resets them to queued so they can be re-claimed.
// Uses the idx_jobs_reaper partial index for efficient scanning.
func (s *ReaperStore) RequeueOrphanedJobs(ctx context.Context) ([]uuid.UUID, error) {
	const q = `
		UPDATE jobs
		SET    status           = 'queued',
		       worker_id        = NULL,
		       claimed_at       = NULL,
		       lease_expires_at = NULL,
		       next_run_at      = NOW(),
		       updated_at       = NOW()
		WHERE  status IN ('claimed', 'running')
		  AND  lease_expires_at < NOW()
		RETURNING id`

	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("requeue orphaned jobs: %w", err)
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan reclaimed job id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// MarkDeadWorkers transitions active workers whose last heartbeat is older
// than staleThreshold to 'offline' status.
func (s *ReaperStore) MarkDeadWorkers(ctx context.Context, staleThreshold time.Duration) (int, error) {
	const q = `
		UPDATE workers
		SET    status     = 'offline',
		       updated_at = NOW()
		WHERE  status = 'active'
		  AND  id IN (
		      SELECT worker_id FROM worker_heartbeats
		      WHERE  last_seen_at < NOW() - $1::interval
		  )`

	tag, err := s.pool.Exec(ctx, q, staleThreshold.String())
	if err != nil {
		return 0, fmt.Errorf("mark dead workers: %w", err)
	}
	return int(tag.RowsAffected()), nil
}
