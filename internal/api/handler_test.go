//go:build integration

package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"codity.ai/scheduler/internal/domain"
	"codity.ai/scheduler/internal/store"
)

// ─── Auth ────────────────────────────────────────────────────────────────────

func TestAuthRegisterAndLogin(t *testing.T) {
	env := setupTestEnv(t)

	tests := []struct {
		name       string
		body       map[string]string
		wantStatus int
		wantCode   string
	}{
		{
			name: "successful registration",
			body: map[string]string{
				"org_name": "Acme", "org_slug": "acme",
				"email": "admin@acme.com", "name": "Admin", "password": "securepass1",
			},
			wantStatus: http.StatusCreated,
		},
		{
			name: "duplicate org slug",
			body: map[string]string{
				"org_name": "Acme2", "org_slug": "acme",
				"email": "admin2@acme.com", "name": "Admin2", "password": "securepass1",
			},
			wantStatus: http.StatusConflict,
			wantCode:   "CONFLICT",
		},
		{
			name: "missing required fields",
			body: map[string]string{
				"org_name": "", "org_slug": "", "email": "", "name": "", "password": "short",
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "VALIDATION_ERROR",
		},
		{
			name: "password too short",
			body: map[string]string{
				"org_name": "Foo", "org_slug": "foo",
				"email": "a@b.com", "name": "A", "password": "short",
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "VALIDATION_ERROR",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := doRequest(env.Server, http.MethodPost, "/auth/register", tc.body, "")
			if rec.Code != tc.wantStatus {
				t.Errorf("status: got %d, want %d (body: %s)", rec.Code, tc.wantStatus, rec.Body.String())
			}
			if tc.wantCode != "" {
				resp := decodeJSON[map[string]any](t, rec)
				if resp["code"] != tc.wantCode {
					t.Errorf("code: got %v, want %s", resp["code"], tc.wantCode)
				}
			}
		})
	}

	// Login with the registered user
	t.Run("login success", func(t *testing.T) {
		rec := doRequest(env.Server, http.MethodPost, "/auth/login",
			map[string]string{"email": "admin@acme.com", "password": "securepass1"}, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("login: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}
		resp := decodeJSON[map[string]any](t, rec)
		if resp["token"] == nil || resp["token"] == "" {
			t.Error("expected non-empty token")
		}
	})

	t.Run("login wrong password", func(t *testing.T) {
		rec := doRequest(env.Server, http.MethodPost, "/auth/login",
			map[string]string{"email": "admin@acme.com", "password": "wrongpass1"}, "")
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("login wrong pw: got %d, want 401", rec.Code)
		}
	})
}

// ─── Projects CRUD ──────────────────────────────────────────────────────────

func TestProjectsCRUD(t *testing.T) {
	env := setupTestEnv(t)
	token := registerAndLogin(t, env.Server, "proj")

	var projectID string

	t.Run("create project", func(t *testing.T) {
		rec := doRequest(env.Server, http.MethodPost, "/projects",
			map[string]string{"name": "Backend", "slug": "backend"}, token)
		if rec.Code != http.StatusCreated {
			t.Fatalf("create: got %d, want 201 (body: %s)", rec.Code, rec.Body.String())
		}
		resp := decodeJSON[domain.Project](t, rec)
		projectID = resp.ID.String()
		if resp.Name != "Backend" {
			t.Errorf("name: got %q, want %q", resp.Name, "Backend")
		}
	})

	t.Run("create duplicate slug", func(t *testing.T) {
		rec := doRequest(env.Server, http.MethodPost, "/projects",
			map[string]string{"name": "Backend2", "slug": "backend"}, token)
		if rec.Code != http.StatusConflict {
			t.Errorf("duplicate slug: got %d, want 409", rec.Code)
		}
	})

	t.Run("get project", func(t *testing.T) {
		rec := doRequest(env.Server, http.MethodGet, "/projects/"+projectID, nil, token)
		if rec.Code != http.StatusOK {
			t.Fatalf("get: got %d, want 200", rec.Code)
		}
		resp := decodeJSON[domain.Project](t, rec)
		if resp.ID.String() != projectID {
			t.Errorf("id mismatch: got %s, want %s", resp.ID, projectID)
		}
	})

	t.Run("list projects", func(t *testing.T) {
		rec := doRequest(env.Server, http.MethodGet, "/projects?limit=10", nil, token)
		if rec.Code != http.StatusOK {
			t.Fatalf("list: got %d, want 200", rec.Code)
		}
		resp := decodeJSON[domain.Page[domain.Project]](t, rec)
		if len(resp.Items) != 1 {
			t.Errorf("items count: got %d, want 1", len(resp.Items))
		}
	})

	t.Run("list projects name filter", func(t *testing.T) {
		rec := doRequest(env.Server, http.MethodGet, "/projects?name=nonexistent", nil, token)
		if rec.Code != http.StatusOK {
			t.Fatalf("list filter: got %d, want 200", rec.Code)
		}
		resp := decodeJSON[domain.Page[domain.Project]](t, rec)
		if len(resp.Items) != 0 {
			t.Errorf("items count: got %d, want 0", len(resp.Items))
		}
	})

	t.Run("update project", func(t *testing.T) {
		newName := "Backend Services"
		rec := doRequest(env.Server, http.MethodPut, "/projects/"+projectID,
			map[string]any{"name": newName}, token)
		if rec.Code != http.StatusOK {
			t.Fatalf("update: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}
		resp := decodeJSON[domain.Project](t, rec)
		if resp.Name != newName {
			t.Errorf("name: got %q, want %q", resp.Name, newName)
		}
	})

	t.Run("delete project", func(t *testing.T) {
		rec := doRequest(env.Server, http.MethodDelete, "/projects/"+projectID, nil, token)
		if rec.Code != http.StatusNoContent {
			t.Errorf("delete: got %d, want 204", rec.Code)
		}
	})

	t.Run("get deleted project returns 404", func(t *testing.T) {
		rec := doRequest(env.Server, http.MethodGet, "/projects/"+projectID, nil, token)
		if rec.Code != http.StatusNotFound {
			t.Errorf("get deleted: got %d, want 404", rec.Code)
		}
	})
}

// ─── Queues CRUD + Pause/Resume ─────────────────────────────────────────────

func TestQueuesCRUD(t *testing.T) {
	env := setupTestEnv(t)
	token := registerAndLogin(t, env.Server, "queue")

	// Create a project first.
	rec := doRequest(env.Server, http.MethodPost, "/projects",
		map[string]string{"name": "Queue Project", "slug": "queue-proj"}, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("setup project: got %d (body: %s)", rec.Code, rec.Body.String())
	}
	proj := decodeJSON[domain.Project](t, rec)

	var queueID string

	t.Run("create queue", func(t *testing.T) {
		body := map[string]any{
			"project_id":       proj.ID.String(),
			"name":             "Emails",
			"slug":             "emails",
			"priority_default": 5,
			"concurrency_limit": 10,
		}
		rec := doRequest(env.Server, http.MethodPost, "/queues", body, token)
		if rec.Code != http.StatusCreated {
			t.Fatalf("create: got %d, want 201 (body: %s)", rec.Code, rec.Body.String())
		}
		resp := decodeJSON[domain.Queue](t, rec)
		queueID = resp.ID.String()
		if resp.PriorityDefault != 5 {
			t.Errorf("priority_default: got %d, want 5", resp.PriorityDefault)
		}
		if resp.ConcurrencyLimit == nil || *resp.ConcurrencyLimit != 10 {
			t.Errorf("concurrency_limit: got %v, want 10", resp.ConcurrencyLimit)
		}
	})

	t.Run("create queue invalid project", func(t *testing.T) {
		body := map[string]any{
			"project_id": "00000000-0000-0000-0000-000000000000",
			"name":       "Bad", "slug": "bad",
		}
		rec := doRequest(env.Server, http.MethodPost, "/queues", body, token)
		if rec.Code != http.StatusNotFound {
			t.Errorf("create bad project: got %d, want 404", rec.Code)
		}
	})

	t.Run("get queue", func(t *testing.T) {
		rec := doRequest(env.Server, http.MethodGet, "/queues/"+queueID, nil, token)
		if rec.Code != http.StatusOK {
			t.Fatalf("get: got %d, want 200", rec.Code)
		}
	})

	t.Run("list queues", func(t *testing.T) {
		rec := doRequest(env.Server, http.MethodGet, "/queues?limit=10", nil, token)
		if rec.Code != http.StatusOK {
			t.Fatalf("list: got %d, want 200", rec.Code)
		}
		resp := decodeJSON[domain.Page[domain.Queue]](t, rec)
		if len(resp.Items) != 1 {
			t.Errorf("items count: got %d, want 1", len(resp.Items))
		}
	})

	t.Run("list queues filter by project_id", func(t *testing.T) {
		rec := doRequest(env.Server, http.MethodGet, "/queues?project_id="+proj.ID.String(), nil, token)
		if rec.Code != http.StatusOK {
			t.Fatalf("list filter: got %d, want 200", rec.Code)
		}
		resp := decodeJSON[domain.Page[domain.Queue]](t, rec)
		if len(resp.Items) != 1 {
			t.Errorf("items count: got %d, want 1", len(resp.Items))
		}
	})

	t.Run("update queue", func(t *testing.T) {
		rec := doRequest(env.Server, http.MethodPut, "/queues/"+queueID,
			map[string]any{"name": "Email Queue", "priority_default": 10}, token)
		if rec.Code != http.StatusOK {
			t.Fatalf("update: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}
		resp := decodeJSON[domain.Queue](t, rec)
		if resp.PriorityDefault != 10 {
			t.Errorf("priority_default: got %d, want 10", resp.PriorityDefault)
		}
	})

	t.Run("pause queue", func(t *testing.T) {
		rec := doRequest(env.Server, http.MethodPost, "/queues/"+queueID+"/pause", nil, token)
		if rec.Code != http.StatusNoContent {
			t.Errorf("pause: got %d, want 204", rec.Code)
		}

		rec = doRequest(env.Server, http.MethodGet, "/queues/"+queueID, nil, token)
		resp := decodeJSON[domain.Queue](t, rec)
		if !resp.IsPaused {
			t.Error("expected queue to be paused")
		}
	})

	t.Run("resume queue", func(t *testing.T) {
		rec := doRequest(env.Server, http.MethodPost, "/queues/"+queueID+"/resume", nil, token)
		if rec.Code != http.StatusNoContent {
			t.Errorf("resume: got %d, want 204", rec.Code)
		}

		rec = doRequest(env.Server, http.MethodGet, "/queues/"+queueID, nil, token)
		resp := decodeJSON[domain.Queue](t, rec)
		if resp.IsPaused {
			t.Error("expected queue to be resumed")
		}
	})

	t.Run("delete queue", func(t *testing.T) {
		rec := doRequest(env.Server, http.MethodDelete, "/queues/"+queueID, nil, token)
		if rec.Code != http.StatusNoContent {
			t.Errorf("delete: got %d, want 204", rec.Code)
		}
	})
}

// ─── Org Scoping (cross-tenant isolation) ───────────────────────────────────

func TestCrossTenantIsolation(t *testing.T) {
	env := setupTestEnv(t)
	tokenA := registerAndLogin(t, env.Server, "tenant-a")
	tokenB := registerAndLogin(t, env.Server, "tenant-b")

	// Tenant A creates a project.
	rec := doRequest(env.Server, http.MethodPost, "/projects",
		map[string]string{"name": "Secret", "slug": "secret"}, tokenA)
	if rec.Code != http.StatusCreated {
		t.Fatalf("setup: got %d", rec.Code)
	}
	proj := decodeJSON[domain.Project](t, rec)

	t.Run("tenant B cannot see tenant A project", func(t *testing.T) {
		rec := doRequest(env.Server, http.MethodGet, "/projects/"+proj.ID.String(), nil, tokenB)
		if rec.Code != http.StatusNotFound {
			t.Errorf("cross-tenant get: got %d, want 404", rec.Code)
		}
	})

	t.Run("tenant B list does not include tenant A projects", func(t *testing.T) {
		rec := doRequest(env.Server, http.MethodGet, "/projects", nil, tokenB)
		if rec.Code != http.StatusOK {
			t.Fatalf("list: got %d", rec.Code)
		}
		resp := decodeJSON[domain.Page[domain.Project]](t, rec)
		for _, p := range resp.Items {
			if p.ID == proj.ID {
				t.Error("tenant B can see tenant A's project in list")
			}
		}
	})

	t.Run("tenant B cannot delete tenant A project", func(t *testing.T) {
		rec := doRequest(env.Server, http.MethodDelete, "/projects/"+proj.ID.String(), nil, tokenB)
		if rec.Code != http.StatusNotFound {
			t.Errorf("cross-tenant delete: got %d, want 404", rec.Code)
		}
	})

	// Tenant A creates a queue; tenant B cannot see it.
	rec = doRequest(env.Server, http.MethodPost, "/queues",
		map[string]any{"project_id": proj.ID.String(), "name": "Private", "slug": "private"}, tokenA)
	if rec.Code != http.StatusCreated {
		t.Fatalf("setup queue: got %d (body: %s)", rec.Code, rec.Body.String())
	}
	q := decodeJSON[domain.Queue](t, rec)

	t.Run("tenant B cannot see tenant A queue", func(t *testing.T) {
		rec := doRequest(env.Server, http.MethodGet, "/queues/"+q.ID.String(), nil, tokenB)
		if rec.Code != http.StatusNotFound {
			t.Errorf("cross-tenant queue get: got %d, want 404", rec.Code)
		}
	})

	t.Run("tenant B cannot pause tenant A queue", func(t *testing.T) {
		rec := doRequest(env.Server, http.MethodPost, "/queues/"+q.ID.String()+"/pause", nil, tokenB)
		if rec.Code != http.StatusNotFound {
			t.Errorf("cross-tenant pause: got %d, want 404", rec.Code)
		}
	})
}

// ─── Job Creation ───────────────────────────────────────────────────────────

// setupQueueForJobs registers an org, creates a project and queue, returns
// (token, queueID) for job tests.
func setupQueueForJobs(t *testing.T, env *testEnv, suffix string) (string, string) {
	t.Helper()
	token := registerAndLogin(t, env.Server, suffix)
	rec := doRequest(env.Server, http.MethodPost, "/projects",
		map[string]string{"name": "Job Project " + suffix, "slug": "job-proj-" + suffix}, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("setup project: %d %s", rec.Code, rec.Body.String())
	}
	proj := decodeJSON[domain.Project](t, rec)

	rec = doRequest(env.Server, http.MethodPost, "/queues",
		map[string]any{
			"project_id":       proj.ID.String(),
			"name":             "Jobs Queue " + suffix,
			"slug":             "jobs-q-" + suffix,
			"priority_default": 5,
		}, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("setup queue: %d %s", rec.Code, rec.Body.String())
	}
	q := decodeJSON[domain.Queue](t, rec)
	return token, q.ID.String()
}

func TestJobCreateImmediate(t *testing.T) {
	env := setupTestEnv(t)
	token, queueID := setupQueueForJobs(t, env, "imm")

	tests := []struct {
		name       string
		body       map[string]any
		wantStatus int
		check      func(t *testing.T, j domain.Job)
	}{
		{
			name:       "immediate job with defaults",
			body:       map[string]any{"payload": map[string]string{"msg": "hello"}},
			wantStatus: http.StatusCreated,
			check: func(t *testing.T, j domain.Job) {
				if j.Status != "queued" {
					t.Errorf("status: got %q, want queued", j.Status)
				}
				if j.Priority != 5 {
					t.Errorf("priority: got %d, want 5 (queue default)", j.Priority)
				}
				if j.NextRunAt == nil {
					t.Error("next_run_at should be set for immediate jobs")
				}
				if j.MaxAttempts != 3 {
					t.Errorf("max_attempts: got %d, want 3 (default)", j.MaxAttempts)
				}
			},
		},
		{
			name: "immediate job with custom priority and max_attempts",
			body: map[string]any{
				"payload":      map[string]string{"msg": "hi"},
				"priority":     10,
				"max_attempts": 5,
			},
			wantStatus: http.StatusCreated,
			check: func(t *testing.T, j domain.Job) {
				if j.Priority != 10 {
					t.Errorf("priority: got %d, want 10", j.Priority)
				}
				if j.MaxAttempts != 5 {
					t.Errorf("max_attempts: got %d, want 5", j.MaxAttempts)
				}
			},
		},
		{
			name:       "missing payload returns 422",
			body:       map[string]any{},
			wantStatus: http.StatusUnprocessableEntity,
		},
		{
			name:       "empty payload returns 422",
			body:       map[string]any{"payload": nil},
			wantStatus: http.StatusUnprocessableEntity,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := doRequest(env.Server, http.MethodPost, "/queues/"+queueID+"/jobs", tc.body, token)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status: got %d, want %d (body: %s)", rec.Code, tc.wantStatus, rec.Body.String())
			}
			if tc.check != nil {
				j := decodeJSON[domain.Job](t, rec)
				tc.check(t, j)
			}
		})
	}
}

func TestJobCreateDelayed(t *testing.T) {
	env := setupTestEnv(t)
	token, queueID := setupQueueForJobs(t, env, "delay")

	tests := []struct {
		name       string
		body       map[string]any
		wantStatus int
		check      func(t *testing.T, j domain.Job)
	}{
		{
			name: "delayed by 60 seconds",
			body: map[string]any{
				"payload":       map[string]string{"type": "delayed"},
				"delay_seconds": 60,
			},
			wantStatus: http.StatusCreated,
			check: func(t *testing.T, j domain.Job) {
				if j.Status != "queued" {
					t.Errorf("status: got %q, want queued", j.Status)
				}
				if j.NextRunAt == nil {
					t.Fatal("next_run_at should be set")
				}
				if j.NextRunAt.Before(time.Now().Add(50 * time.Second)) {
					t.Error("next_run_at should be ~60s in the future")
				}
			},
		},
		{
			name: "delay_seconds <= 0 returns 422",
			body: map[string]any{
				"payload":       map[string]string{"x": "y"},
				"delay_seconds": -1,
			},
			wantStatus: http.StatusUnprocessableEntity,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := doRequest(env.Server, http.MethodPost, "/queues/"+queueID+"/jobs", tc.body, token)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status: got %d, want %d (body: %s)", rec.Code, tc.wantStatus, rec.Body.String())
			}
			if tc.check != nil {
				j := decodeJSON[domain.Job](t, rec)
				tc.check(t, j)
			}
		})
	}
}

func TestJobCreateScheduled(t *testing.T) {
	env := setupTestEnv(t)
	token, queueID := setupQueueForJobs(t, env, "sched")

	futureTime := time.Now().Add(24 * time.Hour).Truncate(time.Second)

	t.Run("scheduled at future timestamp", func(t *testing.T) {
		body := map[string]any{
			"payload": map[string]string{"type": "scheduled"},
			"run_at":  futureTime.Format(time.RFC3339),
		}
		rec := doRequest(env.Server, http.MethodPost, "/queues/"+queueID+"/jobs", body, token)
		if rec.Code != http.StatusCreated {
			t.Fatalf("status: got %d, want 201 (body: %s)", rec.Code, rec.Body.String())
		}
		j := decodeJSON[domain.Job](t, rec)
		if j.Status != "scheduled" {
			t.Errorf("status: got %q, want scheduled", j.Status)
		}
		if j.NextRunAt == nil {
			t.Fatal("next_run_at should be set")
		}
		if j.NextRunAt.Unix() != futureTime.Unix() {
			t.Errorf("next_run_at: got %v, want %v", j.NextRunAt, futureTime)
		}
	})
}

func TestJobCreateRecurring(t *testing.T) {
	env := setupTestEnv(t)
	token, queueID := setupQueueForJobs(t, env, "cron")

	tests := []struct {
		name       string
		body       map[string]any
		wantStatus int
		check      func(t *testing.T, j domain.Job)
	}{
		{
			name: "valid cron expression",
			body: map[string]any{
				"payload":   map[string]string{"type": "recurring"},
				"cron_expr": "*/5 * * * *",
			},
			wantStatus: http.StatusCreated,
			check: func(t *testing.T, j domain.Job) {
				if j.Status != "scheduled" {
					t.Errorf("status: got %q, want scheduled", j.Status)
				}
				if j.CronExpr == nil || *j.CronExpr != "*/5 * * * *" {
					t.Errorf("cron_expr: got %v, want */5 * * * *", j.CronExpr)
				}
				if j.NextRunAt == nil {
					t.Fatal("next_run_at should be computed from cron")
				}
				if j.NextRunAt.Before(time.Now()) {
					t.Error("next_run_at should be in the future")
				}
			},
		},
		{
			name: "invalid cron expression returns 422",
			body: map[string]any{
				"payload":   map[string]string{"x": "y"},
				"cron_expr": "not-a-cron",
			},
			wantStatus: http.StatusUnprocessableEntity,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := doRequest(env.Server, http.MethodPost, "/queues/"+queueID+"/jobs", tc.body, token)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status: got %d, want %d (body: %s)", rec.Code, tc.wantStatus, rec.Body.String())
			}
			if tc.check != nil {
				j := decodeJSON[domain.Job](t, rec)
				tc.check(t, j)
			}
		})
	}
}

func TestJobCreateIdempotency(t *testing.T) {
	env := setupTestEnv(t)
	token, queueID := setupQueueForJobs(t, env, "idemp")

	idemKey := "unique-payment-abc"
	body := map[string]any{
		"payload":         map[string]string{"charge": "$50"},
		"idempotency_key": idemKey,
	}

	t.Run("first create returns 201", func(t *testing.T) {
		rec := doRequest(env.Server, http.MethodPost, "/queues/"+queueID+"/jobs", body, token)
		if rec.Code != http.StatusCreated {
			t.Fatalf("first: got %d, want 201 (body: %s)", rec.Code, rec.Body.String())
		}
		j := decodeJSON[domain.Job](t, rec)
		if j.IdempotencyKey == nil || *j.IdempotencyKey != idemKey {
			t.Errorf("idempotency_key: got %v, want %q", j.IdempotencyKey, idemKey)
		}
	})

	t.Run("second create with same key returns 200 existing job", func(t *testing.T) {
		rec := doRequest(env.Server, http.MethodPost, "/queues/"+queueID+"/jobs", body, token)
		if rec.Code != http.StatusOK {
			t.Fatalf("second: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}
		j := decodeJSON[domain.Job](t, rec)
		if j.IdempotencyKey == nil || *j.IdempotencyKey != idemKey {
			t.Errorf("idempotency_key: got %v, want %q", j.IdempotencyKey, idemKey)
		}
	})

	t.Run("different key creates new job", func(t *testing.T) {
		body2 := map[string]any{
			"payload":         map[string]string{"charge": "$100"},
			"idempotency_key": "different-key-xyz",
		}
		rec := doRequest(env.Server, http.MethodPost, "/queues/"+queueID+"/jobs", body2, token)
		if rec.Code != http.StatusCreated {
			t.Fatalf("different key: got %d, want 201 (body: %s)", rec.Code, rec.Body.String())
		}
	})

	t.Run("no idempotency_key always creates new job", func(t *testing.T) {
		plain := map[string]any{"payload": map[string]string{"msg": "dup-ok"}}
		rec1 := doRequest(env.Server, http.MethodPost, "/queues/"+queueID+"/jobs", plain, token)
		rec2 := doRequest(env.Server, http.MethodPost, "/queues/"+queueID+"/jobs", plain, token)
		if rec1.Code != http.StatusCreated || rec2.Code != http.StatusCreated {
			t.Errorf("no key: both should be 201, got %d and %d", rec1.Code, rec2.Code)
		}
	})
}

func TestJobCreateBatch(t *testing.T) {
	env := setupTestEnv(t)
	token, queueID := setupQueueForJobs(t, env, "batch")

	t.Run("batch of 3 jobs", func(t *testing.T) {
		body := map[string]any{
			"jobs": []map[string]any{
				{"payload": map[string]string{"i": "1"}},
				{"payload": map[string]string{"i": "2"}, "priority": 10},
				{"payload": map[string]string{"i": "3"}, "delay_seconds": 30},
			},
		}
		rec := doRequest(env.Server, http.MethodPost, "/queues/"+queueID+"/jobs/batch", body, token)
		if rec.Code != http.StatusCreated {
			t.Fatalf("batch: got %d, want 201 (body: %s)", rec.Code, rec.Body.String())
		}

		var resp struct {
			BatchID string        `json:"batch_id"`
			Jobs    []domain.Job  `json:"jobs"`
		}
		json.NewDecoder(rec.Body).Decode(&resp) //nolint:errcheck

		if len(resp.Jobs) != 3 {
			t.Fatalf("jobs count: got %d, want 3", len(resp.Jobs))
		}
		if resp.BatchID == "" {
			t.Error("batch_id should be set")
		}
		for _, j := range resp.Jobs {
			if j.BatchID == nil {
				t.Error("each job should have batch_id")
			}
		}
		if resp.Jobs[1].Priority != 10 {
			t.Errorf("job[1] priority: got %d, want 10", resp.Jobs[1].Priority)
		}
	})

	t.Run("empty batch returns 422", func(t *testing.T) {
		rec := doRequest(env.Server, http.MethodPost, "/queues/"+queueID+"/jobs/batch",
			map[string]any{"jobs": []any{}}, token)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("empty batch: got %d, want 422", rec.Code)
		}
	})
}

func TestJobCreateMutualExclusivity(t *testing.T) {
	env := setupTestEnv(t)
	token, queueID := setupQueueForJobs(t, env, "mutex")

	t.Run("delay_seconds + run_at returns 422", func(t *testing.T) {
		body := map[string]any{
			"payload":       map[string]string{"x": "y"},
			"delay_seconds": 60,
			"run_at":        time.Now().Add(time.Hour).Format(time.RFC3339),
		}
		rec := doRequest(env.Server, http.MethodPost, "/queues/"+queueID+"/jobs", body, token)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("mutual exclusivity: got %d, want 422 (body: %s)", rec.Code, rec.Body.String())
		}
	})

	t.Run("delay_seconds + cron_expr returns 422", func(t *testing.T) {
		body := map[string]any{
			"payload":       map[string]string{"x": "y"},
			"delay_seconds": 60,
			"cron_expr":     "*/5 * * * *",
		}
		rec := doRequest(env.Server, http.MethodPost, "/queues/"+queueID+"/jobs", body, token)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("mutual exclusivity: got %d, want 422", rec.Code)
		}
	})
}

// ─── DLQ + Retry ─────────────────────────────────────────────────────────────

func TestDLQTransitionAndManualRetry(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()

	// Setup: org + project + queue with concurrency_limit = 10.
	token := registerAndLogin(t, env.Server, "dlq")
	rec := doRequest(env.Server, http.MethodPost, "/projects",
		map[string]string{"name": "DLQ Project", "slug": "dlq-proj"}, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("setup project: %d", rec.Code)
	}
	proj := decodeJSON[domain.Project](t, rec)

	rec = doRequest(env.Server, http.MethodPost, "/queues",
		map[string]any{
			"project_id":       proj.ID.String(),
			"name":             "DLQ Queue",
			"slug":             "dlq-q",
			"priority_default": 5,
		}, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("setup queue: %d", rec.Code)
	}
	queue := decodeJSON[domain.Queue](t, rec)

	// Create a job with max_attempts=2 so it DLQs after 2 failures.
	rec = doRequest(env.Server, http.MethodPost, "/queues/"+queue.ID.String()+"/jobs",
		map[string]any{
			"payload":      map[string]string{"type": "always_fail"},
			"max_attempts": 2,
		}, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create job: %d %s", rec.Code, rec.Body.String())
	}
	job := decodeJSON[domain.Job](t, rec)

	// Use WorkerStore to simulate the worker failure path directly.
	ws := store.NewWorkerStore(env.Pool)

	workerID, err := ws.RegisterWorker(ctx, "test-dlq-worker")
	if err != nil {
		t.Fatalf("register worker: %v", err)
	}

	t.Run("first failure requeues (attempts < max_attempts)", func(t *testing.T) {
		// Claim the job.
		claimed, err := ws.ClaimJobs(ctx, workerID, queue.ID, 30)
		if err != nil {
			t.Fatalf("claim: %v", err)
		}
		if len(claimed) != 1 {
			t.Fatalf("claimed %d jobs, want 1", len(claimed))
		}

		// Simulate failure: increment attempts, record, requeue.
		newAttempts := claimed[0].Attempts + 1
		if err := ws.FailJob(ctx, job.ID, "test failure 1", newAttempts); err != nil {
			t.Fatalf("fail: %v", err)
		}
		if err := ws.RecordExecution(ctx, &domain.JobExecution{
			JobID: job.ID, WorkerID: workerID, Attempt: newAttempts,
			Status: "failed", StartedAt: time.Now(),
		}); err != nil {
			t.Fatalf("record exec: %v", err)
		}

		// newAttempts (1) < max_attempts (2) → requeue.
		nextRun := time.Now().Add(5 * time.Second)
		if err := ws.RequeueForRetry(ctx, job.ID, nextRun, newAttempts); err != nil {
			t.Fatalf("requeue: %v", err)
		}

		// Verify status is back to queued.
		var status string
		err = env.Pool.QueryRow(ctx, "SELECT status::text FROM jobs WHERE id = $1", job.ID).Scan(&status)
		if err != nil {
			t.Fatalf("query status: %v", err)
		}
		if status != "queued" {
			t.Errorf("status = %s, want queued", status)
		}
	})

	t.Run("second failure moves to DLQ (attempts >= max_attempts)", func(t *testing.T) {
		// Force next_run_at to now so claim works.
		_, err := env.Pool.Exec(ctx, "UPDATE jobs SET next_run_at = NOW() WHERE id = $1", job.ID)
		if err != nil {
			t.Fatalf("update next_run_at: %v", err)
		}

		claimed, err := ws.ClaimJobs(ctx, workerID, queue.ID, 30)
		if err != nil {
			t.Fatalf("claim: %v", err)
		}
		if len(claimed) != 1 {
			t.Fatalf("claimed %d jobs, want 1", len(claimed))
		}

		newAttempts := claimed[0].Attempts + 1 // 1+1 = 2
		if newAttempts < claimed[0].MaxAttempts {
			t.Fatalf("test setup: newAttempts=%d should >= max_attempts=%d", newAttempts, claimed[0].MaxAttempts)
		}

		if err := ws.MoveToDLQ(ctx, job.ID, "test failure 2", newAttempts); err != nil {
			t.Fatalf("move to DLQ: %v", err)
		}

		// Verify job is dead_letter.
		var status string
		err = env.Pool.QueryRow(ctx, "SELECT status::text FROM jobs WHERE id = $1", job.ID).Scan(&status)
		if err != nil {
			t.Fatalf("query status: %v", err)
		}
		if status != "dead_letter" {
			t.Errorf("status = %s, want dead_letter", status)
		}

		// Verify DLQ row exists.
		var dlqReason string
		var dlqAttempts int
		err = env.Pool.QueryRow(ctx,
			"SELECT reason, attempts_made FROM dead_letter_queue WHERE job_id = $1", job.ID,
		).Scan(&dlqReason, &dlqAttempts)
		if err != nil {
			t.Fatalf("query DLQ: %v", err)
		}
		if dlqReason != "test failure 2" {
			t.Errorf("dlq reason = %q, want %q", dlqReason, "test failure 2")
		}
		if dlqAttempts != 2 {
			t.Errorf("dlq attempts = %d, want 2", dlqAttempts)
		}
	})

	t.Run("POST /jobs/:id/retry requeues DLQ job", func(t *testing.T) {
		rec := doRequest(env.Server, http.MethodPost, "/jobs/"+job.ID.String()+"/retry", nil, token)
		if rec.Code != http.StatusOK {
			t.Fatalf("retry: got %d, want 200: %s", rec.Code, rec.Body.String())
		}

		retried := decodeJSON[domain.Job](t, rec)
		if retried.Status != "queued" {
			t.Errorf("retried status = %s, want queued", retried.Status)
		}
		if retried.Attempts != 0 {
			t.Errorf("retried attempts = %d, want 0", retried.Attempts)
		}

		// DLQ row should be gone.
		var count int
		err := env.Pool.QueryRow(ctx,
			"SELECT COUNT(*) FROM dead_letter_queue WHERE job_id = $1", job.ID,
		).Scan(&count)
		if err != nil {
			t.Fatalf("count DLQ: %v", err)
		}
		if count != 0 {
			t.Errorf("DLQ row count = %d, want 0 (should be deleted)", count)
		}
	})

	t.Run("retry non-DLQ job returns 404", func(t *testing.T) {
		// The job is now in 'queued' status, not dead_letter.
		rec := doRequest(env.Server, http.MethodPost, "/jobs/"+job.ID.String()+"/retry", nil, token)
		if rec.Code != http.StatusNotFound {
			t.Errorf("retry non-DLQ: got %d, want 404", rec.Code)
		}
	})

	t.Run("retry nonexistent job returns 404", func(t *testing.T) {
		fakeID := uuid.New().String()
		rec := doRequest(env.Server, http.MethodPost, "/jobs/"+fakeID+"/retry", nil, token)
		if rec.Code != http.StatusNotFound {
			t.Errorf("retry nonexistent: got %d, want 404", rec.Code)
		}
	})
}

// ─── Middleware ──────────────────────────────────────────────────────────────

func TestMiddleware(t *testing.T) {
	env := setupTestEnv(t)

	t.Run("unauthenticated request returns 401", func(t *testing.T) {
		rec := doRequest(env.Server, http.MethodGet, "/projects", nil, "")
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("no auth: got %d, want 401", rec.Code)
		}
	})

	t.Run("invalid token returns 401", func(t *testing.T) {
		rec := doRequest(env.Server, http.MethodGet, "/projects", nil, "bad.token.value")
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("bad token: got %d, want 401", rec.Code)
		}
	})

	t.Run("request ID header is set", func(t *testing.T) {
		rec := doRequest(env.Server, http.MethodGet, "/healthz", nil, "")
		if rec.Header().Get("X-Request-ID") == "" {
			t.Error("expected X-Request-ID header")
		}
	})

	t.Run("health check returns 200", func(t *testing.T) {
		rec := doRequest(env.Server, http.MethodGet, "/healthz", nil, "")
		if rec.Code != http.StatusOK {
			t.Errorf("healthz: got %d, want 200", rec.Code)
		}
	})
}
