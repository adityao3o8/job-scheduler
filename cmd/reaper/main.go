package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"codity.ai/scheduler/internal/reaper"
	"codity.ai/scheduler/internal/store"
)

type config struct {
	DatabaseURL    string
	Interval       time.Duration
	StaleThreshold time.Duration
}

func loadConfig() config {
	return config{
		DatabaseURL:    envOr("DATABASE_URL", "postgres://localhost:5432/scheduler?sslmode=disable"),
		Interval:       envDuration("REAPER_INTERVAL", 10*time.Second),
		StaleThreshold: envDuration("STALE_THRESHOLD", 30*time.Second),
	}
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg := loadConfig()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("connect to database", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		logger.Error("ping database", slog.String("error", err.Error()))
		os.Exit(1)
	}

	r := reaper.New(store.NewReaperStore(pool), logger, reaper.Config{
		Interval:       cfg.Interval,
		StaleThreshold: cfg.StaleThreshold,
	})

	r.Run(ctx)
	logger.Info("reaper exited")
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return d
}
