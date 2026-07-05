//go:build integration

package store_test

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"codity.ai/scheduler/internal/store"
)

// ── Test helpers ────────────────────────────────────────────────────────────

func setupPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = startPG(t, ctx)
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close)

	applyMigrations(t, ctx, pool)
	return pool
}

func startPG(t *testing.T, ctx context.Context) string {
	t.Helper()
	ctr, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "postgres:16-alpine",
			ExposedPorts: []string{"5432/tcp"},
			Env: map[string]string{
				"POSTGRES_USER": "test", "POSTGRES_PASSWORD": "test",
				"POSTGRES_DB": "scheduler_test",
			},
			WaitingFor: wait.ForListeningPort("5432/tcp").WithStartupTimeout(30 * time.Second),
		},
		Started: true,
	})
	if err != nil {
		t.Fatalf("start pg: %v", err)
	}
	t.Cleanup(func() { ctr.Terminate(ctx) }) //nolint:errcheck

	host, _ := ctr.Host(ctx)
	port, _ := ctr.MappedPort(ctx, "5432")
	return fmt.Sprintf("postgres://test:test@%s:%s/scheduler_test?sslmode=disable", host, port.Port())
}

func applyMigrations(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	dir := filepath.Join("..", "..", "migrations")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read migrations: %v", err)
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".sql" {
			files = append(files, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(files)
	for _, f := range files {
		raw, _ := os.ReadFile(f)
		sql := extractUp(string(raw))
		if sql == "" {
			continue
		}
		if _, err := pool.Exec(ctx, sql); err != nil {
			t.Fatalf("migration %s: %v", filepath.Base(f), err)
		}
	}
}

func extractUp(content string) string {
	const up = "-- +goose Up"
	const down = "-- +goose Down"
	s := strings.Index(content, up)
	if s < 0 {
		return ""
	}
	s += len(up)
	e := strings.Index(content, down)
	if e < 0 {
		e = len(content)
	}
	return content[s:e]
}

// seedQueue creates an org → project → queue with the given concurrency limit,
// then inserts jobCount queued jobs with next_run_at = NOW(). Returns the queue ID.
func seedQueue(t *testing.T, ctx context.Context, pool *pgxpool.Pool, concurrencyLimit *int, jobCount int) uuid.UUID {
	t.Helper()
	orgID := uuid.New()
	projID := uuid.New()
	queueID := uuid.New()

	mustExec(t, ctx, pool,
		`INSERT INTO organizations (id, name, slug) VALUES ($1, $2, $3)`,
		orgID, "Test Org", "test-org-"+orgID.String()[:8])

	mustExec(t, ctx, pool,
		`INSERT INTO projects (id, org_id, name, slug) VALUES ($1, $2, $3, $4)`,
		projID, orgID, "Test Project", "test-proj-"+projID.String()[:8])

	mustExec(t, ctx, pool,
		`INSERT INTO queues (id, project_id, name, slug, concurrency_limit)
		 VALUES ($1, $2, $3, $4, $5)`,
		queueID, projID, "Test Queue", "test-queue-"+queueID.String()[:8], concurrencyLimit)

	for i := range jobCount {
		mustExec(t, ctx, pool,
			`INSERT INTO jobs (id, queue_id, status, priority, payload, next_run_at)
			 VALUES ($1, $2, 'queued', $3, '{"i":'||$4||'}', NOW())`,
			uuid.New(), queueID, jobCount-i, i)
	}

	return queueID
}

func mustExec(t *testing.T, ctx context.Context, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(ctx, sql, args...); err != nil {
		t.Fatalf("exec: %v\nSQL: %s", err, sql)
	}
}

// ── Tests ───────────────────────────────────────────────────────────────────

func TestClaimJobs_ConcurrentWorkers_NoDuplicates(t *testing.T) {
	pool := setupPool(t)
	ctx := context.Background()

	const (
		concurrency = 5
		totalJobs   = 20
		numWorkers  = 10
	)
	limit := concurrency
	queueID := seedQueue(t, ctx, pool, &limit, totalJobs)
	repo := store.NewJobRepository(pool)

	// Spin up numWorkers goroutines all claiming simultaneously.
	type result struct {
		workerID uuid.UUID
		claimed  []uuid.UUID
		err      error
	}
	results := make([]result, numWorkers)
	var wg sync.WaitGroup
	// Use a barrier so all goroutines start at the same instant.
	barrier := make(chan struct{})

	for i := range numWorkers {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			wID := uuid.New()
			<-barrier
			jobs, err := repo.ClaimJobs(ctx, wID, queueID, 30)
			ids := make([]uuid.UUID, len(jobs))
			for j, job := range jobs {
				ids[j] = job.ID
			}
			results[idx] = result{workerID: wID, claimed: ids, err: err}
		}(i)
	}
	close(barrier) // release all goroutines at once
	wg.Wait()

	// Collect all claimed IDs.
	seen := make(map[uuid.UUID]uuid.UUID) // job ID → worker who claimed it
	totalClaimed := 0
	for _, r := range results {
		if r.err != nil {
			t.Fatalf("worker %s error: %v", r.workerID, r.err)
		}
		for _, jID := range r.claimed {
			if prev, dup := seen[jID]; dup {
				t.Errorf("DUPLICATE: job %s claimed by both %s and %s", jID, prev, r.workerID)
			}
			seen[jID] = r.workerID
		}
		totalClaimed += len(r.claimed)
	}

	if totalClaimed != concurrency {
		t.Errorf("total claimed: got %d, want %d (concurrency_limit)", totalClaimed, concurrency)
	}

	// Verify DB state: exactly `concurrency` jobs in claimed status.
	var dbClaimed int
	pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM jobs WHERE queue_id = $1 AND status = 'claimed'`,
		queueID).Scan(&dbClaimed)
	if dbClaimed != concurrency {
		t.Errorf("DB claimed count: got %d, want %d", dbClaimed, concurrency)
	}

	t.Logf("SUCCESS: %d workers competed, %d total claimed, 0 duplicates", numWorkers, totalClaimed)
}

func TestClaimJobs_UnlimitedConcurrency(t *testing.T) {
	pool := setupPool(t)
	ctx := context.Background()

	const totalJobs = 15
	// nil concurrency_limit → unlimited (capped at INT_MAX internally)
	queueID := seedQueue(t, ctx, pool, nil, totalJobs)
	repo := store.NewJobRepository(pool)

	wID := uuid.New()
	jobs, err := repo.ClaimJobs(ctx, wID, queueID, 30)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if len(jobs) != totalJobs {
		t.Errorf("claimed: got %d, want %d (all, since no concurrency limit)", len(jobs), totalJobs)
	}
}

func TestClaimJobs_PriorityOrder(t *testing.T) {
	pool := setupPool(t)
	ctx := context.Background()

	limit := 100
	queueID := seedQueue(t, ctx, pool, &limit, 5)
	repo := store.NewJobRepository(pool)

	jobs, err := repo.ClaimJobs(ctx, uuid.New(), queueID, 30)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}

	// seedQueue sets priority = jobCount - i, so the first job inserted has
	// the highest priority. Verify descending priority order.
	for i := 1; i < len(jobs); i++ {
		if jobs[i].Priority > jobs[i-1].Priority {
			t.Errorf("jobs not in priority DESC order: job[%d].priority=%d > job[%d].priority=%d",
				i, jobs[i].Priority, i-1, jobs[i-1].Priority)
		}
	}
}

func TestClaimJobs_BudgetExhausted(t *testing.T) {
	pool := setupPool(t)
	ctx := context.Background()

	limit := 3
	queueID := seedQueue(t, ctx, pool, &limit, 10)
	repo := store.NewJobRepository(pool)

	// First claim takes all 3 slots.
	jobs1, err := repo.ClaimJobs(ctx, uuid.New(), queueID, 30)
	if err != nil {
		t.Fatalf("first claim: %v", err)
	}
	if len(jobs1) != 3 {
		t.Fatalf("first claim: got %d, want 3", len(jobs1))
	}

	// Second claim: budget is exhausted (3 claimed, limit 3).
	jobs2, err := repo.ClaimJobs(ctx, uuid.New(), queueID, 30)
	if err != nil {
		t.Fatalf("second claim: %v", err)
	}
	if len(jobs2) != 0 {
		t.Errorf("second claim: got %d, want 0 (budget exhausted)", len(jobs2))
	}
}

func TestClaimJobs_ExplainAnalyze_UsesPartialIndex(t *testing.T) {
	pool := setupPool(t)
	ctx := context.Background()

	limit := 100
	queueID := seedQueue(t, ctx, pool, &limit, 50)

	// Run EXPLAIN ANALYZE on the inner SELECT of the claim query.
	const explainQ = `
		EXPLAIN ANALYZE
		SELECT id FROM jobs
		WHERE  queue_id = $1
		  AND  status = 'queued'
		  AND  next_run_at <= NOW()
		ORDER  BY priority DESC, next_run_at ASC
		FOR UPDATE SKIP LOCKED
		LIMIT  10`

	rows, err := pool.Query(ctx, explainQ, queueID)
	if err != nil {
		t.Fatalf("EXPLAIN ANALYZE: %v", err)
	}
	defer rows.Close()

	var plan strings.Builder
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatalf("scan explain line: %v", err)
		}
		plan.WriteString(line)
		plan.WriteByte('\n')
	}

	output := plan.String()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	logger.Info("EXPLAIN ANALYZE output", slog.String("plan", output))

	// The planner must use the partial index idx_jobs_claim, not a seq scan.
	if !strings.Contains(output, "idx_jobs_claim") {
		t.Errorf("EXPLAIN ANALYZE does not reference idx_jobs_claim.\n"+
			"Expected the partial index to be used. Full plan:\n%s", output)
	}

	// Must not contain Seq Scan on the jobs table for the inner query.
	if strings.Contains(output, "Seq Scan on jobs") {
		t.Errorf("EXPLAIN ANALYZE shows Seq Scan — the partial index is not being used.\n"+
			"Full plan:\n%s", output)
	}

	t.Logf("EXPLAIN ANALYZE confirms idx_jobs_claim is used:\n%s", output)
}
