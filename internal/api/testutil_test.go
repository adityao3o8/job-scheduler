//go:build integration

package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"codity.ai/scheduler/internal/api"
	"codity.ai/scheduler/internal/store"
	"codity.ai/scheduler/pkg/auth"
)

const testJWTSecret = "test-secret-key-for-testing-only"

// testEnv holds a running test server backed by a real Postgres.
type testEnv struct {
	Server *api.Server
	Pool   *pgxpool.Pool
	JWT    *auth.JWTService
}

func setupTestEnv(t *testing.T) *testEnv {
	t.Helper()
	ctx := context.Background()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = startPostgresContainer(t, ctx)
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close)

	applyMigrations(t, ctx, pool)

	jwtSvc := auth.NewJWTService(testJWTSecret, time.Hour)
	srv := api.NewServer(api.ServerDeps{
		Logger:   slog.New(slog.NewJSONHandler(os.Stdout, nil)),
		JWT:      jwtSvc,
		Orgs:     store.NewOrgRepository(pool),
		Users:    store.NewUserRepository(pool),
		Projects: store.NewProjectRepository(pool),
		Queues:   store.NewQueueRepository(pool),
		Jobs:     store.NewJobRepository(pool),
		Tx:       &store.TxManager{Pool: pool},
		Metrics:  store.NewMetricsStore(pool),
	})

	return &testEnv{Server: srv, Pool: pool, JWT: jwtSvc}
}

func startPostgresContainer(t *testing.T, ctx context.Context) string {
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
		t.Fatalf("start postgres container: %v", err)
	}
	t.Cleanup(func() { container.Terminate(ctx) }) //nolint:errcheck

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("container host: %v", err)
	}
	port, err := container.MappedPort(ctx, "5432")
	if err != nil {
		t.Fatalf("container port: %v", err)
	}

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

// extractGooseUp returns the SQL between "-- +goose Up" and "-- +goose Down".
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

// ── HTTP helpers ────────────────────────────────────────────────────────────

func doRequest(srv http.Handler, method, path string, body any, token string) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if body != nil {
		json.NewEncoder(&buf).Encode(body) //nolint:errcheck
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	return w
}

func decodeJSON[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.NewDecoder(rec.Body).Decode(&v); err != nil {
		t.Fatalf("decode response: %v (body=%s)", err, rec.Body.String())
	}
	return v
}

// registerAndLogin creates a fresh org+user and returns the auth token.
func registerAndLogin(t *testing.T, srv http.Handler, suffix string) string {
	t.Helper()
	body := map[string]string{
		"org_name": "Test Org " + suffix,
		"org_slug": "test-org-" + suffix,
		"email":    suffix + "@test.com",
		"name":     "Test User",
		"password": "password123",
	}
	rec := doRequest(srv, http.MethodPost, "/auth/register", body, "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("register: want 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct{ Token string }
	json.NewDecoder(rec.Body).Decode(&resp) //nolint:errcheck
	return resp.Token
}
