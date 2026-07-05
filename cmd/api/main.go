package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/prometheus/client_golang/prometheus"

	"codity.ai/scheduler/internal/api"
	"codity.ai/scheduler/internal/reaper"
	"codity.ai/scheduler/internal/store"
	"codity.ai/scheduler/pkg/auth"
	"codity.ai/scheduler/pkg/metrics"
)

type config struct {
	Port           string
	DatabaseURL    string
	JWTSecret      string
	JWTTTL         time.Duration
	EmbedReaper    bool
	DemoAuth       bool
	ReaperInterval time.Duration
	StaleThreshold time.Duration
}

func loadConfig() config {
	c := config{
		Port:           envOr("PORT", "8080"),
		DatabaseURL:    envOr("DATABASE_URL", "postgres://localhost:5432/scheduler?sslmode=disable"),
		JWTSecret:      envOr("JWT_SECRET", "change-me-in-production"),
		JWTTTL:         24 * time.Hour,
		EmbedReaper:    os.Getenv("EMBED_REAPER") == "true",
		DemoAuth:       os.Getenv("DEMO_AUTH") == "true",
		ReaperInterval: 10 * time.Second,
		StaleThreshold: 30 * time.Second,
	}
	if v := os.Getenv("JWT_TTL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			c.JWTTTL = d
		}
	}
	if v := os.Getenv("REAPER_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			c.ReaperInterval = d
		}
	}
	if v := os.Getenv("STALE_THRESHOLD"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			c.StaleThreshold = d
		}
	}
	return c
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg := loadConfig()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("failed to connect to database", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		logger.Error("failed to ping database", slog.String("error", err.Error()))
		os.Exit(1)
	}

	jwtSvc := auth.NewJWTService(cfg.JWTSecret, cfg.JWTTTL)
	metricsStore := store.NewMetricsStore(pool)

	prometheus.MustRegister(metrics.NewDBCollector(metricsStore, logger))

	srv := api.NewServer(api.ServerDeps{
		Logger:   logger,
		JWT:      jwtSvc,
		DemoAuth: cfg.DemoAuth,
		Orgs:     store.NewOrgRepository(pool),
		Users:    store.NewUserRepository(pool),
		Projects: store.NewProjectRepository(pool),
		Queues:   store.NewQueueRepository(pool),
		Jobs:     store.NewJobRepository(pool),
		Tx:       &store.TxManager{Pool: pool},
		Metrics:  metricsStore,
	})

	httpSrv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           srv,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		logger.Info("api server starting", slog.String("addr", httpSrv.Addr))
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server error", slog.String("error", err.Error()))
			cancel()
		}
	}()

	if cfg.EmbedReaper {
		rp := reaper.New(store.NewReaperStore(pool), logger, reaper.Config{
			Interval:       cfg.ReaperInterval,
			StaleThreshold: cfg.StaleThreshold,
		})
		go rp.Run(ctx)
		logger.Info("embedded reaper started")
	}

	<-ctx.Done()
	logger.Info("shutting down gracefully")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()

	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown error", slog.String("error", err.Error()))
	}
	logger.Info("server stopped")
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
