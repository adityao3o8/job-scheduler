package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"codity.ai/scheduler/internal/domain"
	"codity.ai/scheduler/pkg/pagination"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}

func readJSON(r *http.Request, dst any) error {
	if r.Body == nil {
		return fmt.Errorf("empty request body: %w", domain.ErrValidation)
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("invalid JSON: %s: %w", err, domain.ErrValidation)
	}
	return nil
}

func pageParams(r *http.Request) domain.PageParams {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	return domain.PageParams{
		Limit:  pagination.ClampLimit(limit),
		Cursor: r.URL.Query().Get("cursor"),
	}
}
