package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"codity.ai/scheduler/internal/domain"
)

type OrgHandler struct {
	orgs domain.OrgRepository
}

func NewOrgHandler(orgs domain.OrgRepository) *OrgHandler {
	return &OrgHandler{orgs: orgs}
}

func (h *OrgHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid org id", nil)
		return
	}

	claims := claimsFromCtx(r.Context())
	if claims.OrgID != id {
		writeError(w, http.StatusForbidden, "FORBIDDEN", "cross-tenant access denied", nil)
		return
	}

	org, err := h.orgs.GetByID(r.Context(), id)
	if err != nil {
		mapDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, org)
}

type updateOrgRequest struct {
	Name *string `json:"name"`
	Slug *string `json:"slug"`
}

func (h *OrgHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid org id", nil)
		return
	}

	claims := claimsFromCtx(r.Context())
	if claims.OrgID != id {
		writeError(w, http.StatusForbidden, "FORBIDDEN", "cross-tenant access denied", nil)
		return
	}

	var req updateOrgRequest
	if err := readJSON(r, &req); err != nil {
		mapDomainError(w, err)
		return
	}

	org, err := h.orgs.GetByID(r.Context(), id)
	if err != nil {
		mapDomainError(w, err)
		return
	}

	if req.Name != nil {
		if *req.Name == "" {
			writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "name cannot be empty", nil)
			return
		}
		org.Name = *req.Name
	}
	if req.Slug != nil {
		if !slugRe.MatchString(*req.Slug) {
			writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "invalid slug format", nil)
			return
		}
		org.Slug = *req.Slug
	}

	if err := h.orgs.Update(r.Context(), org); err != nil {
		mapDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, org)
}

func (h *OrgHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid org id", nil)
		return
	}

	claims := claimsFromCtx(r.Context())
	if claims.OrgID != id {
		writeError(w, http.StatusForbidden, "FORBIDDEN", "cross-tenant access denied", nil)
		return
	}

	if err := h.orgs.Delete(r.Context(), id); err != nil {
		mapDomainError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
