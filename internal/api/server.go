package api

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"codity.ai/scheduler/internal/domain"
	"codity.ai/scheduler/pkg/auth"
)

type Server struct {
	router   chi.Router
	logger   *slog.Logger
	jwt      *auth.JWTService
	demoAuth bool
	orgs     domain.OrgRepository
	users    domain.UserRepository
	projects domain.ProjectRepository
	queues   domain.QueueRepository
	jobs     domain.JobRepository
	tx       domain.TxManager
	metrics  domain.MetricsStore
}

type ServerDeps struct {
	Logger   *slog.Logger
	JWT      *auth.JWTService
	DemoAuth bool
	Orgs     domain.OrgRepository
	Users    domain.UserRepository
	Projects domain.ProjectRepository
	Queues   domain.QueueRepository
	Jobs     domain.JobRepository
	Tx       domain.TxManager
	Metrics  domain.MetricsStore
}

func NewServer(deps ServerDeps) *Server {
	s := &Server{
		router:   chi.NewRouter(),
		logger:   deps.Logger,
		jwt:      deps.JWT,
		demoAuth: deps.DemoAuth,
		orgs:     deps.Orgs,
		users:    deps.Users,
		projects: deps.Projects,
		queues:   deps.Queues,
		jobs:     deps.Jobs,
		tx:       deps.Tx,
		metrics:  deps.Metrics,
	}
	s.routes()
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

func (s *Server) routes() {
	r := s.router

	// Global middleware stack
	r.Use(RequestID)
	r.Use(PanicRecovery(s.logger))
	r.Use(RequestLogger(s.logger))

	// Unauthenticated operational endpoints
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	r.Handle("/metrics", promhttp.Handler())

	// Auth (unauthenticated)
	authH := NewAuthHandler(s.users, s.orgs, s.tx, s.jwt, s.demoAuth)
	r.Post("/auth/register", authH.Register)
	r.Post("/auth/login", authH.Login)

	// Authenticated routes
	r.Group(func(r chi.Router) {
		r.Use(Auth(s.jwt))

		// Organizations
		orgH := NewOrgHandler(s.orgs)
		r.Get("/orgs/{id}", orgH.Get)
		r.Put("/orgs/{id}", orgH.Update)
		r.Delete("/orgs/{id}", orgH.Delete)

		// Projects
		projH := NewProjectHandler(s.projects)
		r.Get("/projects", projH.List)
		r.Post("/projects", projH.Create)
		r.Get("/projects/{id}", projH.Get)
		r.Put("/projects/{id}", projH.Update)
		r.Delete("/projects/{id}", projH.Delete)

		// Queues
		qH := NewQueueHandler(s.queues, s.projects)
		r.Get("/queues", qH.List)
		r.Post("/queues", qH.Create)
		r.Get("/queues/{id}", qH.Get)
		r.Put("/queues/{id}", qH.Update)
		r.Delete("/queues/{id}", qH.Delete)
		r.Post("/queues/{id}/pause", qH.Pause)
		r.Post("/queues/{id}/resume", qH.Resume)

		// Jobs (nested under queues)
		jobH := NewJobHandler(s.jobs, s.queues, s.tx)
		r.Post("/queues/{id}/jobs", jobH.Create)
		r.Post("/queues/{id}/jobs/batch", jobH.CreateBatch)
		r.Post("/jobs/{id}/retry", jobH.Retry)
		r.Get("/jobs", jobH.ListJobs)
		r.Get("/jobs/{id}", jobH.Get)
		r.Get("/jobs/{id}/executions", jobH.Executions)
		r.Get("/jobs/{id}/logs", jobH.Logs)
		r.Get("/queues/{id}/jobs", jobH.ListByQueue)
		r.Get("/dlq", jobH.ListDLQ)

		// Stats / observability
		statsH := NewStatsHandler(s.metrics, s.queues)
		r.Get("/queues/{id}/stats", statsH.QueueStats)
		r.Get("/workers", statsH.ListWorkers)
	})
}
