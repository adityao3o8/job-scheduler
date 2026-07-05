package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/robfig/cron/v3"

	"codity.ai/scheduler/internal/domain"
	"codity.ai/scheduler/pkg/metrics"
)

const maxPayloadBytes = 256 * 1024 // 256 KB

var cronParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)

type JobHandler struct {
	jobs   domain.JobRepository
	queues domain.QueueRepository
	tx     domain.TxManager
}

func NewJobHandler(jobs domain.JobRepository, queues domain.QueueRepository, tx domain.TxManager) *JobHandler {
	return &JobHandler{jobs: jobs, queues: queues, tx: tx}
}

type createJobRequest struct {
	Payload        json.RawMessage `json:"payload"`
	Priority       *int            `json:"priority,omitempty"`
	MaxAttempts    *int            `json:"max_attempts,omitempty"`
	IdempotencyKey *string         `json:"idempotency_key,omitempty"`
	DelaySeconds   *int            `json:"delay_seconds,omitempty"`
	RunAt          *time.Time      `json:"run_at,omitempty"`
	CronExpr       *string         `json:"cron_expr,omitempty"`
}

// Create handles POST /queues/{id}/jobs for all job types:
// immediate, delayed, scheduled, and recurring.
func (h *JobHandler) Create(w http.ResponseWriter, r *http.Request) {
	queueID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid queue id", nil)
		return
	}

	var req createJobRequest
	if err := readJSON(r, &req); err != nil {
		mapDomainError(w, err)
		return
	}

	claims := claimsFromCtx(r.Context())
	queue, err := h.queues.GetByID(r.Context(), queueID, claims.OrgID)
	if err != nil {
		mapDomainError(w, err)
		return
	}

	input, valErrs := buildJobInput(req, queue)
	if len(valErrs) > 0 {
		writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "invalid input", valErrs)
		return
	}

	result, err := h.jobs.Create(r.Context(), input)
	if err != nil {
		mapDomainError(w, err)
		return
	}

	if result.Created {
		metrics.JobsSubmitted.Inc()
		writeJSON(w, http.StatusCreated, result.Job)
	} else {
		writeJSON(w, http.StatusOK, result.Job)
	}
}

type createJobBatchRequest struct {
	Jobs []createJobRequest `json:"jobs"`
}

type batchResponse struct {
	BatchID uuid.UUID    `json:"batch_id"`
	Jobs    []*domain.Job `json:"jobs"`
}

// CreateBatch handles POST /queues/{id}/jobs/batch.
// All jobs share one batch_id and are inserted in a single transaction.
func (h *JobHandler) CreateBatch(w http.ResponseWriter, r *http.Request) {
	queueID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid queue id", nil)
		return
	}

	var req createJobBatchRequest
	if err := readJSON(r, &req); err != nil {
		mapDomainError(w, err)
		return
	}
	if len(req.Jobs) == 0 {
		writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "jobs array is empty", nil)
		return
	}
	if len(req.Jobs) > 1000 {
		writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "batch limited to 1000 jobs", nil)
		return
	}

	claims := claimsFromCtx(r.Context())
	queue, err := h.queues.GetByID(r.Context(), queueID, claims.OrgID)
	if err != nil {
		mapDomainError(w, err)
		return
	}

	batchID := uuid.New()
	inputs := make([]*domain.CreateJobInput, 0, len(req.Jobs))

	for i, jr := range req.Jobs {
		input, valErrs := buildJobInput(jr, queue)
		if len(valErrs) > 0 {
			writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR",
				"invalid input at index "+itoa(i), valErrs)
			return
		}
		input.BatchID = &batchID
		inputs = append(inputs, input)
	}

	results := make([]*domain.Job, 0, len(inputs))
	err = h.tx.WithTx(r.Context(), func(ctx context.Context) error {
		for _, input := range inputs {
			res, err := h.jobs.Create(ctx, input)
			if err != nil {
				return err
			}
			results = append(results, res.Job)
		}
		return nil
	})
	if err != nil {
		mapDomainError(w, err)
		return
	}

	metrics.JobsSubmitted.Add(float64(len(results)))
	writeJSON(w, http.StatusCreated, batchResponse{BatchID: batchID, Jobs: results})
}

// buildJobInput validates a single job request and constructs the CreateJobInput.
// Returns validation errors if any field is invalid.
func buildJobInput(req createJobRequest, queue *domain.Queue) (*domain.CreateJobInput, map[string]string) {
	errs := make(map[string]string)

	// Payload validation
	if len(req.Payload) == 0 {
		errs["payload"] = "required"
	} else if len(req.Payload) > maxPayloadBytes {
		errs["payload"] = "exceeds 256KB size limit"
	} else if !json.Valid(req.Payload) {
		errs["payload"] = "must be valid JSON"
	}

	// Mutual exclusivity of scheduling modes
	modes := 0
	if req.DelaySeconds != nil {
		modes++
	}
	if req.RunAt != nil {
		modes++
	}
	if req.CronExpr != nil {
		modes++
	}
	if modes > 1 {
		errs["scheduling"] = "only one of delay_seconds, run_at, cron_expr allowed"
	}

	if req.DelaySeconds != nil && *req.DelaySeconds <= 0 {
		errs["delay_seconds"] = "must be positive"
	}
	if req.RunAt != nil && req.RunAt.Before(time.Now()) {
		errs["run_at"] = "must be in the future"
	}
	if req.CronExpr != nil {
		if _, err := cronParser.Parse(*req.CronExpr); err != nil {
			errs["cron_expr"] = "invalid cron expression: " + err.Error()
		}
	}
	if req.MaxAttempts != nil && *req.MaxAttempts < 1 {
		errs["max_attempts"] = "must be at least 1"
	}

	if len(errs) > 0 {
		return nil, errs
	}

	// Determine status and next_run_at based on scheduling type.
	status := "queued"
	var nextRunAt *time.Time

	switch {
	case req.CronExpr != nil:
		status = "scheduled"
		sched, _ := cronParser.Parse(*req.CronExpr) // already validated above
		next := sched.Next(time.Now())
		nextRunAt = &next

	case req.RunAt != nil:
		status = "scheduled"
		nextRunAt = req.RunAt

	case req.DelaySeconds != nil:
		t := time.Now().Add(time.Duration(*req.DelaySeconds) * time.Second)
		nextRunAt = &t

	default:
		// Immediate: next_run_at = now so the claim query picks it up instantly.
		t := time.Now()
		nextRunAt = &t
	}

	priority := queue.PriorityDefault
	if req.Priority != nil {
		priority = *req.Priority
	}

	maxAttempts := 3
	if req.MaxAttempts != nil {
		maxAttempts = *req.MaxAttempts
	}

	return &domain.CreateJobInput{
		QueueID:        queue.ID,
		Priority:       priority,
		Payload:        req.Payload,
		IdempotencyKey: req.IdempotencyKey,
		NextRunAt:      nextRunAt,
		Status:         status,
		MaxAttempts:    maxAttempts,
		CronExpr:       req.CronExpr,
	}, nil
}

// ListJobs handles GET /jobs.
func (h *JobHandler) ListJobs(w http.ResponseWriter, r *http.Request) {
	claims := claimsFromCtx(r.Context())
	status := r.URL.Query().Get("status")

	var queueID *uuid.UUID
	if v := r.URL.Query().Get("queue_id"); v != "" {
		parsed, err := uuid.Parse(v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid queue_id", nil)
			return
		}
		queueID = &parsed
	}

	page, err := h.jobs.ListAll(r.Context(), claims.OrgID, status, queueID, pageParams(r))
	if err != nil {
		mapDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

// ListByQueue handles GET /queues/{id}/jobs.
func (h *JobHandler) ListByQueue(w http.ResponseWriter, r *http.Request) {
	queueID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid queue id", nil)
		return
	}

	claims := claimsFromCtx(r.Context())
	status := r.URL.Query().Get("status")

	page, err := h.jobs.ListByQueue(r.Context(), queueID, claims.OrgID, status, pageParams(r))
	if err != nil {
		mapDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

// Get handles GET /jobs/{id}.
func (h *JobHandler) Get(w http.ResponseWriter, r *http.Request) {
	jobID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid job id", nil)
		return
	}

	claims := claimsFromCtx(r.Context())
	job, err := h.jobs.GetByID(r.Context(), jobID, claims.OrgID)
	if err != nil {
		mapDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, job)
}

// Executions handles GET /jobs/{id}/executions.
func (h *JobHandler) Executions(w http.ResponseWriter, r *http.Request) {
	jobID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid job id", nil)
		return
	}

	claims := claimsFromCtx(r.Context())
	execs, err := h.jobs.GetExecutions(r.Context(), jobID, claims.OrgID)
	if err != nil {
		mapDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, execs)
}

// Logs handles GET /jobs/{id}/logs.
func (h *JobHandler) Logs(w http.ResponseWriter, r *http.Request) {
	jobID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid job id", nil)
		return
	}

	claims := claimsFromCtx(r.Context())
	logs, err := h.jobs.GetLogs(r.Context(), jobID, claims.OrgID)
	if err != nil {
		mapDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, logs)
}

// ListDLQ handles GET /dlq.
func (h *JobHandler) ListDLQ(w http.ResponseWriter, r *http.Request) {
	claims := claimsFromCtx(r.Context())
	page, err := h.jobs.ListDLQ(r.Context(), claims.OrgID, pageParams(r))
	if err != nil {
		mapDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

// Retry handles POST /jobs/{id}/retry — requeues a dead-letter job.
func (h *JobHandler) Retry(w http.ResponseWriter, r *http.Request) {
	jobID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid job id", nil)
		return
	}

	claims := claimsFromCtx(r.Context())
	job, err := h.jobs.RetryDLQJob(r.Context(), jobID, claims.OrgID)
	if err != nil {
		mapDomainError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, job)
}

func itoa(i int) string {
	const digits = "0123456789"
	if i < 10 {
		return string(digits[i])
	}
	return itoa(i/10) + string(digits[i%10])
}
