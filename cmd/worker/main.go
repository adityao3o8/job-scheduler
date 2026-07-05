package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"codity.ai/scheduler/internal/store"
	"codity.ai/scheduler/internal/worker"
)

type config struct {
	DatabaseURL       string
	WorkerName        string
	MaxConcurrency    int
	PollInterval      time.Duration
	LeaseSeconds      int
	HeartbeatInterval time.Duration
	DrainTimeout      time.Duration
}

func loadConfig() config {
	return config{
		DatabaseURL:       envOr("DATABASE_URL", "postgres://localhost:5432/scheduler?sslmode=disable"),
		WorkerName:        envOr("WORKER_NAME", hostname()),
		MaxConcurrency:    envInt("MAX_CONCURRENCY", 10),
		PollInterval:      envDuration("POLL_INTERVAL", 2*time.Second),
		LeaseSeconds:      envInt("LEASE_SECONDS", 30),
		HeartbeatInterval: envDuration("HEARTBEAT_INTERVAL", 10*time.Second),
		DrainTimeout:      envDuration("DRAIN_TIMEOUT", 30*time.Second),
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

	ws := store.NewWorkerStore(pool)

	engine := worker.NewEngine(ws, logger, worker.Config{
		WorkerName:        cfg.WorkerName,
		MaxConcurrency:    cfg.MaxConcurrency,
		PollInterval:      cfg.PollInterval,
		LeaseSeconds:      cfg.LeaseSeconds,
		HeartbeatInterval: cfg.HeartbeatInterval,
		DrainTimeout:      cfg.DrainTimeout,
	})

	engine.RegisterHandler("http_call", &worker.HTTPCallHandler{
		Client: &http.Client{Timeout: 30 * time.Second},
	})
	engine.RegisterHandler("sleep", &worker.SleepHandler{})
	engine.RegisterHandler("always_fail", &worker.AlwaysFailHandler{})

	logger.Info("starting worker",
		slog.String("name", cfg.WorkerName),
		slog.Int("max_concurrency", cfg.MaxConcurrency),
		slog.Duration("poll_interval", cfg.PollInterval))

	if err := engine.Run(ctx); err != nil {
		logger.Error("worker error", slog.String("error", err.Error()))
		os.Exit(1)
	}
	logger.Info("worker stopped")
}

func hostname() string {
	h, err := os.Hostname()
	if err != nil {
		return "worker-unknown"
	}
	return h
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
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
