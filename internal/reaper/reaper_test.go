//go:build integration

package reaper_test

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"codity.ai/scheduler/internal/reaper"
	"codity.ai/scheduler/internal/store"
)

func setupPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = startPostgres(t, ctx)
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close)

	applyMigrations(t, ctx, pool)
	return pool
}

func startPostgres(t *testing.T, ctx context.Context) string {
	t.Helper()
	req := testcontainers.ContainerRequest{
		Image:        "postgres:16-alpine",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_USER":     "test",
			"POSTGRES_PASSWORD": "test",
			"POSTGRES_DB":       "scheduler_test",
		},
		WaitingFor: wait.ForListeningPort("5432/tcp").WithStartupTimeout(30 * time.Second),
	}
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req, Started: true,
	})
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	t.Cleanup(func() { container.Terminate(ctx) }) //nolint:errcheck

	host, _ := container.Host(ctx)
	port, _ := container.MappedPort(ctx, "5432")
	return fmt.Sprintf("postgres://test:test@%s:%s/scheduler_test?sslmode=disable", host, port.Port())
}

func applyMigrations(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	migrationsDir := filepath.Join("..", "..", "migrations")
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		t.Fatalf("read migrations dir: %v", err)
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".sql" {
			files = append(files, filepath.Join(migrationsDir, e.Name()))
		}
	}
	sort.Strings(files)
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read migration %s: %v", f, err)
		}
		sql := extractGooseUp(string(raw))
		if sql == "" {
			continue
		}
		if _, err := pool.Exec(ctx, sql); err != nil {
			t.Fatalf("apply migration %s: %v", filepath.Base(f), err)
		}
	}
}

func extractGooseUp(content string) string {
	const upMarker = "-- +goose Up"
	const downMarker = "-- +goose Down"
	start := 0
	for i := range len(content) - len(upMarker) {
		if content[i:i+len(upMarker)] == upMarker {
			start = i + len(upMarker)
			break
		}
	}
	end := len(content)
	for i := range len(content) - len(downMarker) {
		if content[i:i+len(downMarker)] == downMarker {
			end = i
			break
		}
	}
	if start >= end {
		return ""
	}
	return content[start:end]
}

func TestReaper_RequeuesOrphanedJob(t *testing.T) {
	pool := setupPool(t)
	ctx := context.Background()

	// Seed an org → project → queue chain.
	orgID := uuid.New()
	projID := uuid.New()
	queueID := uuid.New()

	_, err := pool.Exec(ctx, `INSERT INTO organizations (id, name, slug) VALUES ($1, 'Reaper Org', 'reaper-org')`, orgID)
	if err != nil {
		t.Fatalf("insert org: %v", err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO projects (id, org_id, name, slug) VALUES ($1, $2, 'Reaper Proj', 'reaper-proj')`, projID, orgID)
	if err != nil {
		t.Fatalf("insert project: %v", err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO queues (id, project_id, name, slug) VALUES ($1, $2, 'Reaper Queue', 'reaper-q')`, queueID, projID)
	if err != nil {
		t.Fatalf("insert queue: %v", err)
	}

	// Create a worker with a stale heartbeat.
	workerID := uuid.New()
	_, err = pool.Exec(ctx, `INSERT INTO workers (id, name, status) VALUES ($1, 'dead-worker', 'active')`, workerID)
	if err != nil {
		t.Fatalf("insert worker: %v", err)
	}
	_, err = pool.Exec(ctx,
		`INSERT INTO worker_heartbeats (worker_id, last_seen_at) VALUES ($1, NOW() - interval '5 minutes')`,
		workerID)
	if err != nil {
		t.Fatalf("insert heartbeat: %v", err)
	}

	// Insert a job in 'running' status with an expired lease, assigned to the
	// dead worker. This simulates a worker that was OOM-killed mid-execution.
	jobID := uuid.New()
	_, err = pool.Exec(ctx, `
		INSERT INTO jobs (id, queue_id, status, priority, payload, worker_id,
			claimed_at, lease_expires_at, next_run_at)
		VALUES ($1, $2, 'running', 5, '{"type":"test"}'::jsonb, $3,
			NOW() - interval '2 minutes',
			NOW() - interval '1 minute',
			NOW() - interval '2 minutes')`,
		jobID, queueID, workerID)
	if err != nil {
		t.Fatalf("insert orphaned job: %v", err)
	}

	// Run a single reaper sweep.
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	r := reaper.New(store.NewReaperStore(pool), logger, reaper.Config{
		Interval:       10 * time.Second,
		StaleThreshold: 30 * time.Second,
	})
	r.RunOnce(ctx)

	// Assert the job is back to queued.
	var status string
	var workerIDResult *uuid.UUID
	var leaseExpires *time.Time
	err = pool.QueryRow(ctx,
		`SELECT status::text, worker_id, lease_expires_at FROM jobs WHERE id = $1`, jobID,
	).Scan(&status, &workerIDResult, &leaseExpires)
	if err != nil {
		t.Fatalf("query job: %v", err)
	}
	if status != "queued" {
		t.Errorf("job status = %s, want queued", status)
	}
	if workerIDResult != nil {
		t.Errorf("worker_id = %v, want nil", workerIDResult)
	}
	if leaseExpires != nil {
		t.Errorf("lease_expires_at = %v, want nil", leaseExpires)
	}

	// Assert the worker is marked offline (dead).
	var workerStatus string
	err = pool.QueryRow(ctx, `SELECT status::text FROM workers WHERE id = $1`, workerID).Scan(&workerStatus)
	if err != nil {
		t.Fatalf("query worker: %v", err)
	}
	if workerStatus != "offline" {
		t.Errorf("worker status = %s, want offline", workerStatus)
	}
}

func TestReaper_SkipsNonExpiredLease(t *testing.T) {
	pool := setupPool(t)
	ctx := context.Background()

	orgID := uuid.New()
	projID := uuid.New()
	queueID := uuid.New()
	workerID := uuid.New()

	pool.Exec(ctx, `INSERT INTO organizations (id, name, slug) VALUES ($1, 'OK Org', 'ok-org')`, orgID)                             //nolint:errcheck
	pool.Exec(ctx, `INSERT INTO projects (id, org_id, name, slug) VALUES ($1, $2, 'OK Proj', 'ok-proj')`, projID, orgID)              //nolint:errcheck
	pool.Exec(ctx, `INSERT INTO queues (id, project_id, name, slug) VALUES ($1, $2, 'OK Queue', 'ok-q')`, queueID, projID)            //nolint:errcheck
	pool.Exec(ctx, `INSERT INTO workers (id, name, status) VALUES ($1, 'alive-worker', 'active')`, workerID)                          //nolint:errcheck
	pool.Exec(ctx, `INSERT INTO worker_heartbeats (worker_id, last_seen_at) VALUES ($1, NOW())`, workerID)                            //nolint:errcheck

	// Job with a lease that expires in the future — should NOT be reaped.
	jobID := uuid.New()
	pool.Exec(ctx, `
		INSERT INTO jobs (id, queue_id, status, priority, payload, worker_id,
			claimed_at, lease_expires_at, next_run_at)
		VALUES ($1, $2, 'running', 5, '{"type":"test"}'::jsonb, $3,
			NOW(), NOW() + interval '5 minutes', NOW())`,
		jobID, queueID, workerID) //nolint:errcheck

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	r := reaper.New(store.NewReaperStore(pool), logger, reaper.Config{
		Interval:       10 * time.Second,
		StaleThreshold: 30 * time.Second,
	})
	r.RunOnce(ctx)

	var status string
	pool.QueryRow(ctx, `SELECT status::text FROM jobs WHERE id = $1`, jobID).Scan(&status) //nolint:errcheck
	if status != "running" {
		t.Errorf("job status = %s, want running (should not be reaped)", status)
	}

	var workerStatus string
	pool.QueryRow(ctx, `SELECT status::text FROM workers WHERE id = $1`, workerID).Scan(&workerStatus) //nolint:errcheck
	if workerStatus != "active" {
		t.Errorf("worker status = %s, want active (heartbeat is fresh)", workerStatus)
	}
}
