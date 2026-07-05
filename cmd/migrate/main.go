package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL is required")
	}

	ctx := context.Background()

	var pool *pgxpool.Pool
	var err error
	for attempt := 1; attempt <= 30; attempt++ {
		pool, err = pgxpool.New(ctx, dsn)
		if err == nil {
			if err = pool.Ping(ctx); err == nil {
				break
			}
		}
		log.Printf("waiting for postgres (attempt %d/30): %v", attempt, err)
		time.Sleep(time.Second)
	}
	if err != nil {
		log.Fatalf("could not connect to postgres after 30 attempts: %v", err)
	}
	defer pool.Close()

	if _, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			filename TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`); err != nil {
		log.Fatalf("ensure schema_migrations: %v", err)
	}

	dir := "/migrations"
	if d := os.Getenv("MIGRATIONS_DIR"); d != "" {
		dir = d
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		log.Fatalf("read migrations dir %s: %v", dir, err)
	}

	var files []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".sql" {
			files = append(files, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(files)

	if err := bootstrapExistingSchema(ctx, pool, files); err != nil {
		log.Fatalf("bootstrap migrations: %v", err)
	}

	for _, f := range files {
		name := filepath.Base(f)
		var exists bool
		if err := pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE filename = $1)`, name,
		).Scan(&exists); err != nil {
			log.Fatalf("check %s: %v", name, err)
		}
		if exists {
			fmt.Printf("skip (already applied): %s\n", name)
			continue
		}

		raw, err := os.ReadFile(f)
		if err != nil {
			log.Fatalf("read %s: %v", f, err)
		}
		sql := extractUp(string(raw))
		if sql == "" {
			continue
		}
		if _, err := pool.Exec(ctx, sql); err != nil {
			log.Fatalf("apply %s: %v", name, err)
		}
		if _, err := pool.Exec(ctx,
			`INSERT INTO schema_migrations (filename) VALUES ($1)`, name,
		); err != nil {
			log.Fatalf("record %s: %v", name, err)
		}
		fmt.Printf("applied: %s\n", name)
	}
	fmt.Println("all migrations applied")
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
	if s >= e {
		return ""
	}
	return content[s:e]
}

// bootstrapExistingSchema marks all migrations applied when the DB was created
// before schema_migrations tracking existed.
func bootstrapExistingSchema(ctx context.Context, pool *pgxpool.Pool, files []string) error {
	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	var jobsExists bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = 'public' AND table_name = 'jobs'
		)`).Scan(&jobsExists); err != nil {
		return err
	}
	if !jobsExists {
		return nil
	}
	for _, f := range files {
		name := filepath.Base(f)
		if _, err := pool.Exec(ctx,
			`INSERT INTO schema_migrations (filename) VALUES ($1) ON CONFLICT DO NOTHING`, name,
		); err != nil {
			return fmt.Errorf("bootstrap %s: %w", name, err)
		}
		fmt.Printf("bootstrapped (pre-existing schema): %s\n", name)
	}
	return nil
}
