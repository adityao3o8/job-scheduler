package worker_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"codity.ai/scheduler/internal/domain"
	"codity.ai/scheduler/internal/worker"
)

// ── Mock store ──────────────────────────────────────────────────────────────

type mockWorkerStore struct {
	mu sync.Mutex

	workerID uuid.UUID
	queues   []domain.Queue
	jobs     []domain.Job

	claimReturned atomic.Bool // ensures jobs are only returned once
	runningJobs   []uuid.UUID
	completedJobs []uuid.UUID
	releaseCalled bool
	deregistered  bool
}

func (m *mockWorkerStore) RegisterWorker(_ context.Context, _ string) (uuid.UUID, error) {
	return m.workerID, nil
}

func (m *mockWorkerStore) DeregisterWorker(_ context.Context, _ uuid.UUID) error {
	m.mu.Lock()
	m.deregistered = true
	m.mu.Unlock()
	return nil
}

func (m *mockWorkerStore) ListActiveQueues(_ context.Context) ([]domain.Queue, error) {
	return m.queues, nil
}

func (m *mockWorkerStore) ClaimJobs(_ context.Context, _, _ uuid.UUID, _ int) ([]domain.Job, error) {
	if m.claimReturned.CompareAndSwap(false, true) {
		return m.jobs, nil
	}
	return nil, nil
}

func (m *mockWorkerStore) SetJobRunning(_ context.Context, jobID uuid.UUID) error {
	m.mu.Lock()
	m.runningJobs = append(m.runningJobs, jobID)
	m.mu.Unlock()
	return nil
}

func (m *mockWorkerStore) CompleteJob(_ context.Context, jobID uuid.UUID) error {
	m.mu.Lock()
	m.completedJobs = append(m.completedJobs, jobID)
	m.mu.Unlock()
	return nil
}

func (m *mockWorkerStore) FailJob(_ context.Context, _ uuid.UUID, _ string, _ int) error {
	return nil
}
func (m *mockWorkerStore) RequeueForRetry(_ context.Context, _ uuid.UUID, _ time.Time, _ int) error {
	return nil
}
func (m *mockWorkerStore) MoveToDLQ(_ context.Context, _ uuid.UUID, _ string, _ int) error {
	return nil
}
func (m *mockWorkerStore) GetRetryPolicy(_ context.Context, _ uuid.UUID) (*domain.RetryPolicy, error) {
	return nil, domain.ErrNotFound
}

func (m *mockWorkerStore) ReleaseJobs(_ context.Context, _ uuid.UUID) (int, error) {
	m.mu.Lock()
	m.releaseCalled = true
	m.mu.Unlock()
	return 0, nil
}

func (m *mockWorkerStore) IsIdempotencyKeyCompleted(_ context.Context, _ string) (bool, error) {
	return false, nil
}
func (m *mockWorkerStore) RecordExecution(_ context.Context, _ *domain.JobExecution) error {
	return nil
}
func (m *mockWorkerStore) AppendJobLog(_ context.Context, _ *domain.JobLogEntry) error {
	return nil
}
func (m *mockWorkerStore) Heartbeat(_ context.Context, _ uuid.UUID, _ int) error {
	return nil
}

// ── Test handler ────────────────────────────────────────────────────────────

type blockingHandler struct {
	started  chan uuid.UUID
	duration time.Duration
}

func (h *blockingHandler) Handle(_ context.Context, job domain.Job) error {
	h.started <- job.ID
	time.Sleep(h.duration)
	return nil
}

// ── Tests ───────────────────────────────────────────────────────────────────

func TestGracefulShutdown_DrainsInFlightWork(t *testing.T) {
	queueID := uuid.New()
	job1 := domain.Job{
		ID: uuid.New(), QueueID: queueID, Status: "claimed",
		Payload: json.RawMessage(`{"type":"block"}`), MaxAttempts: 3,
	}
	job2 := domain.Job{
		ID: uuid.New(), QueueID: queueID, Status: "claimed",
		Payload: json.RawMessage(`{"type":"block"}`), MaxAttempts: 3,
	}

	ms := &mockWorkerStore{
		workerID: uuid.New(),
		queues:   []domain.Queue{{ID: queueID, Name: "test-q"}},
		jobs:     []domain.Job{job1, job2},
	}

	started := make(chan uuid.UUID, 2)

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
	eng := worker.NewEngine(ms, logger, worker.Config{
		WorkerName:        "shutdown-test",
		MaxConcurrency:    4,
		PollInterval:      50 * time.Millisecond,
		LeaseSeconds:      30,
		HeartbeatInterval: time.Hour,
		DrainTimeout:      5 * time.Second,
	})

	eng.RegisterHandler("block", &blockingHandler{
		started:  started,
		duration: 300 * time.Millisecond,
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- eng.Run(ctx) }()

	// Wait for both jobs to begin executing.
	<-started
	<-started

	// Simulate SIGTERM — cancel context while jobs are still running.
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("engine.Run returned error: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("engine.Run did not return within 10s — drain stalled")
	}

	ms.mu.Lock()
	defer ms.mu.Unlock()

	if len(ms.completedJobs) != 2 {
		t.Errorf("completed jobs: got %d, want 2 (in-flight work must drain)", len(ms.completedJobs))
	}
	if !ms.releaseCalled {
		t.Error("ReleaseJobs was not called during shutdown")
	}
	if !ms.deregistered {
		t.Error("DeregisterWorker was not called during shutdown")
	}
}

func TestGracefulShutdown_DrainTimeoutForcesExit(t *testing.T) {
	queueID := uuid.New()
	job := domain.Job{
		ID: uuid.New(), QueueID: queueID, Status: "claimed",
		Payload: json.RawMessage(`{"type":"block"}`), MaxAttempts: 3,
	}

	ms := &mockWorkerStore{
		workerID: uuid.New(),
		queues:   []domain.Queue{{ID: queueID, Name: "test-q"}},
		jobs:     []domain.Job{job},
	}

	started := make(chan uuid.UUID, 1)
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
	eng := worker.NewEngine(ms, logger, worker.Config{
		WorkerName:        "timeout-test",
		MaxConcurrency:    2,
		PollInterval:      50 * time.Millisecond,
		LeaseSeconds:      30,
		HeartbeatInterval: time.Hour,
		DrainTimeout:      200 * time.Millisecond, // very short timeout
	})

	eng.RegisterHandler("block", &blockingHandler{
		started:  started,
		duration: 5 * time.Second, // way longer than drain timeout
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- eng.Run(ctx) }()

	<-started
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("engine.Run returned error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("engine.Run did not exit within 3s — drain timeout not enforced")
	}

	// Even though the job didn't finish, the engine must still deregister.
	ms.mu.Lock()
	defer ms.mu.Unlock()
	if !ms.deregistered {
		t.Error("DeregisterWorker was not called after drain timeout")
	}
}

func TestGracefulShutdown_NoJobsExitsCleanly(t *testing.T) {
	ms := &mockWorkerStore{
		workerID: uuid.New(),
		queues:   []domain.Queue{{ID: uuid.New(), Name: "empty-q"}},
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
	eng := worker.NewEngine(ms, logger, worker.Config{
		WorkerName:        "empty-test",
		MaxConcurrency:    2,
		PollInterval:      50 * time.Millisecond,
		LeaseSeconds:      30,
		HeartbeatInterval: time.Hour,
		DrainTimeout:      time.Second,
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- eng.Run(ctx) }()

	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("engine.Run returned error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("engine.Run did not exit cleanly with no jobs")
	}

	ms.mu.Lock()
	defer ms.mu.Unlock()
	if !ms.deregistered {
		t.Error("DeregisterWorker was not called on clean exit")
	}
	if !ms.releaseCalled {
		t.Error("ReleaseJobs was not called on clean exit")
	}
}
