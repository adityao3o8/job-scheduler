package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"codity.ai/scheduler/internal/domain"
)

type ProjectHandler struct {
	projects domain.ProjectRepository
}

func NewProjectHandler(projects domain.ProjectRepository) *ProjectHandler {
	return &ProjectHandler{projects: projects}
}

type createProjectRequest struct {
	Name        string  `json:"name"`
	Slug        string  `json:"slug"`
	Description *string `json:"description,omitempty"`
}

func (h *ProjectHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createProjectRequest
	if err := readJSON(r, &req); err != nil {
		mapDomainError(w, err)
		return
	}

	if errs := validateCreateProject(req); len(errs) > 0 {
		writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "invalid input", errs)
		return
	}

	claims := claimsFromCtx(r.Context())
	p := &domain.Project{
		OrgID:       claims.OrgID,
		Name:        req.Name,
		Slug:        req.Slug,
		Description: req.Description,
	}
	if err := h.projects.Create(r.Context(), p); err != nil {
		mapDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func (h *ProjectHandler) List(w http.ResponseWriter, r *http.Request) {
	claims := claimsFromCtx(r.Context())
	nameFilter := r.URL.Query().Get("name")

	page, err := h.projects.List(r.Context(), claims.OrgID, pageParams(r), nameFilter)
	if err != nil {
		mapDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (h *ProjectHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid project id", nil)
		return
	}

	claims := claimsFromCtx(r.Context())
	p, err := h.projects.GetByID(r.Context(), id, claims.OrgID)
	if err != nil {
		mapDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

type updateProjectRequest struct {
	Name        *string `json:"name"`
	Slug        *string `json:"slug"`
	Description *string `json:"description"`
}

func (h *ProjectHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid project id", nil)
		return
	}

	var req updateProjectRequest
	if err := readJSON(r, &req); err != nil {
		mapDomainError(w, err)
		return
	}

	claims := claimsFromCtx(r.Context())
	p, err := h.projects.GetByID(r.Context(), id, claims.OrgID)
	if err != nil {
		mapDomainError(w, err)
		return
	}

	if req.Name != nil {
		if *req.Name == "" {
			writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "name cannot be empty", nil)
			return
		}
		p.Name = *req.Name
	}
	if req.Slug != nil {
		if !slugRe.MatchString(*req.Slug) {
			writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "invalid slug format", nil)
			return
		}
		p.Slug = *req.Slug
	}
	if req.Description != nil {
		p.Description = req.Description
	}

	if err := h.projects.Update(r.Context(), p); err != nil {
		mapDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (h *ProjectHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid project id", nil)
		return
	}

	claims := claimsFromCtx(r.Context())
	if err := h.projects.Delete(r.Context(), id, claims.OrgID); err != nil {
		mapDomainError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func validateCreateProject(r createProjectRequest) map[string]string {
	errs := make(map[string]string)
	if r.Name == "" {
		errs["name"] = "required"
	}
	if !slugRe.MatchString(r.Slug) {
		errs["slug"] = "must be lowercase alphanumeric with hyphens"
	}
	return errs
}
