package api

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"codity.ai/scheduler/internal/domain"
)

type QueueHandler struct {
	queues   domain.QueueRepository
	projects domain.ProjectRepository
}

func NewQueueHandler(queues domain.QueueRepository, projects domain.ProjectRepository) *QueueHandler {
	return &QueueHandler{queues: queues, projects: projects}
}

type createQueueRequest struct {
	ProjectID        string  `json:"project_id"`
	Name             string  `json:"name"`
	Slug             string  `json:"slug"`
	RetryPolicyID    *string `json:"retry_policy_id,omitempty"`
	PriorityDefault  *int    `json:"priority_default,omitempty"`
	ConcurrencyLimit *int    `json:"concurrency_limit,omitempty"`
	IsPaused         *bool   `json:"is_paused,omitempty"`
}

func (h *QueueHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createQueueRequest
	if err := readJSON(r, &req); err != nil {
		mapDomainError(w, err)
		return
	}

	if errs := validateCreateQueue(req); len(errs) > 0 {
		writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "invalid input", errs)
		return
	}

	claims := claimsFromCtx(r.Context())
	projectID, _ := uuid.Parse(req.ProjectID)

	// Verify the project belongs to the caller's org.
	if _, err := h.projects.GetByID(r.Context(), projectID, claims.OrgID); err != nil {
		mapDomainError(w, err)
		return
	}

	q := &domain.Queue{
		ProjectID: projectID,
		Name:      req.Name,
		Slug:      req.Slug,
	}
	if req.RetryPolicyID != nil {
		rpID, err := uuid.Parse(*req.RetryPolicyID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid retry_policy_id", nil)
			return
		}
		q.RetryPolicyID = &rpID
	}
	if req.PriorityDefault != nil {
		q.PriorityDefault = *req.PriorityDefault
	}
	if req.ConcurrencyLimit != nil {
		q.ConcurrencyLimit = req.ConcurrencyLimit
	}
	if req.IsPaused != nil {
		q.IsPaused = *req.IsPaused
	}

	if err := h.queues.Create(r.Context(), q); err != nil {
		mapDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, q)
}

func (h *QueueHandler) List(w http.ResponseWriter, r *http.Request) {
	claims := claimsFromCtx(r.Context())
	filters := domain.QueueFilters{Name: r.URL.Query().Get("name")}

	if v := r.URL.Query().Get("is_paused"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "is_paused must be true or false", nil)
			return
		}
		filters.IsPaused = &b
	}
	if v := r.URL.Query().Get("project_id"); v != "" {
		pid, err := uuid.Parse(v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid project_id filter", nil)
			return
		}
		filters.ProjectID = &pid
	}

	page, err := h.queues.List(r.Context(), claims.OrgID, pageParams(r), filters)
	if err != nil {
		mapDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (h *QueueHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid queue id", nil)
		return
	}

	claims := claimsFromCtx(r.Context())
	q, err := h.queues.GetByID(r.Context(), id, claims.OrgID)
	if err != nil {
		mapDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, q)
}

type updateQueueRequest struct {
	Name             *string `json:"name"`
	Slug             *string `json:"slug"`
	RetryPolicyID    *string `json:"retry_policy_id"`
	PriorityDefault  *int    `json:"priority_default"`
	ConcurrencyLimit *int    `json:"concurrency_limit"`
	IsPaused         *bool   `json:"is_paused"`
}

func (h *QueueHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid queue id", nil)
		return
	}

	var req updateQueueRequest
	if err := readJSON(r, &req); err != nil {
		mapDomainError(w, err)
		return
	}

	claims := claimsFromCtx(r.Context())
	q, err := h.queues.GetByID(r.Context(), id, claims.OrgID)
	if err != nil {
		mapDomainError(w, err)
		return
	}

	if req.Name != nil {
		if *req.Name == "" {
			writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "name cannot be empty", nil)
			return
		}
		q.Name = *req.Name
	}
	if req.Slug != nil {
		if !slugRe.MatchString(*req.Slug) {
			writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "invalid slug format", nil)
			return
		}
		q.Slug = *req.Slug
	}
	if req.RetryPolicyID != nil {
		rpID, perr := uuid.Parse(*req.RetryPolicyID)
		if perr != nil {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid retry_policy_id", nil)
			return
		}
		q.RetryPolicyID = &rpID
	}
	if req.PriorityDefault != nil {
		q.PriorityDefault = *req.PriorityDefault
	}
	if req.ConcurrencyLimit != nil {
		q.ConcurrencyLimit = req.ConcurrencyLimit
	}
	if req.IsPaused != nil {
		q.IsPaused = *req.IsPaused
	}

	if err := h.queues.Update(r.Context(), q, claims.OrgID); err != nil {
		mapDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, q)
}

func (h *QueueHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid queue id", nil)
		return
	}

	claims := claimsFromCtx(r.Context())
	if err := h.queues.Delete(r.Context(), id, claims.OrgID); err != nil {
		mapDomainError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *QueueHandler) Pause(w http.ResponseWriter, r *http.Request) {
	h.setPaused(w, r, true)
}

func (h *QueueHandler) Resume(w http.ResponseWriter, r *http.Request) {
	h.setPaused(w, r, false)
}

func (h *QueueHandler) setPaused(w http.ResponseWriter, r *http.Request, paused bool) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid queue id", nil)
		return
	}

	claims := claimsFromCtx(r.Context())
	if err := h.queues.SetPaused(r.Context(), id, claims.OrgID, paused); err != nil {
		mapDomainError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func validateCreateQueue(r createQueueRequest) map[string]string {
	errs := make(map[string]string)
	if _, err := uuid.Parse(r.ProjectID); err != nil {
		errs["project_id"] = "must be a valid UUID"
	}
	if r.Name == "" {
		errs["name"] = "required"
	}
	if !slugRe.MatchString(r.Slug) {
		errs["slug"] = "must be lowercase alphanumeric with hyphens"
	}
	return errs
}
