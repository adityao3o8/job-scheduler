package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"codity.ai/scheduler/internal/domain"
)

type StatsHandler struct {
	metrics domain.MetricsStore
	queues  domain.QueueRepository
}

func NewStatsHandler(metrics domain.MetricsStore, queues domain.QueueRepository) *StatsHandler {
	return &StatsHandler{metrics: metrics, queues: queues}
}

// QueueStats handles GET /queues/{id}/stats — returns depth, throughput,
// success rate, average latency, and p95 latency.
func (h *StatsHandler) QueueStats(w http.ResponseWriter, r *http.Request) {
	queueID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid queue id", nil)
		return
	}

	claims := claimsFromCtx(r.Context())
	if _, err := h.queues.GetByID(r.Context(), queueID, claims.OrgID); err != nil {
		mapDomainError(w, err)
		return
	}

	stats, err := h.metrics.GetQueueStats(r.Context(), queueID)
	if err != nil {
		mapDomainError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, stats)
}

// ListWorkers handles GET /workers — returns all workers with status,
// heartbeat, and in-flight job count.
func (h *StatsHandler) ListWorkers(w http.ResponseWriter, r *http.Request) {
	workers, err := h.metrics.ListWorkers(r.Context())
	if err != nil {
		mapDomainError(w, err)
		return
	}
	if workers == nil {
		workers = []domain.WorkerInfo{}
	}
	writeJSON(w, http.StatusOK, workers)
}
