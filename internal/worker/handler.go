package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"codity.ai/scheduler/internal/domain"
)

// JobHandler processes a single job. Implementations must be safe for
// concurrent use and respect ctx cancellation.
type JobHandler interface {
	Handle(ctx context.Context, job domain.Job) error
}

// ── http_call ───────────────────────────────────────────────────────────────
// Payload: {"type":"http_call","url":"...","method":"POST","headers":{...},"body":"..."}

type HTTPCallHandler struct {
	Client *http.Client
}

func (h *HTTPCallHandler) Handle(ctx context.Context, job domain.Job) error {
	var p struct {
		URL     string            `json:"url"`
		Method  string            `json:"method"`
		Headers map[string]string `json:"headers"`
		Body    string            `json:"body"`
	}
	if err := json.Unmarshal(job.Payload, &p); err != nil {
		return fmt.Errorf("parse http_call payload: %w", err)
	}
	if p.URL == "" {
		return fmt.Errorf("http_call: url is required")
	}
	if p.Method == "" {
		p.Method = http.MethodGet
	}

	req, err := http.NewRequestWithContext(ctx, p.Method, p.URL, strings.NewReader(p.Body))
	if err != nil {
		return fmt.Errorf("http_call build request: %w", err)
	}
	for k, v := range p.Headers {
		req.Header.Set(k, v)
	}

	resp, err := h.Client.Do(req)
	if err != nil {
		return fmt.Errorf("http_call: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body) //nolint:errcheck

	if resp.StatusCode >= 400 {
		return fmt.Errorf("http_call: server returned %d", resp.StatusCode)
	}
	return nil
}

// ── sleep ───────────────────────────────────────────────────────────────────
// Payload: {"type":"sleep","duration_ms":500}

type SleepHandler struct{}

func (h *SleepHandler) Handle(ctx context.Context, job domain.Job) error {
	var p struct {
		DurationMs int `json:"duration_ms"`
	}
	if err := json.Unmarshal(job.Payload, &p); err != nil {
		return fmt.Errorf("parse sleep payload: %w", err)
	}
	if p.DurationMs <= 0 {
		p.DurationMs = 100
	}

	select {
	case <-time.After(time.Duration(p.DurationMs) * time.Millisecond):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// ── always_fail ─────────────────────────────────────────────────────────────
// Payload: {"type":"always_fail","message":"optional custom message"}

type AlwaysFailHandler struct{}

func (h *AlwaysFailHandler) Handle(_ context.Context, job domain.Job) error {
	var p struct {
		Message string `json:"message"`
	}
	json.Unmarshal(job.Payload, &p) //nolint:errcheck
	if p.Message == "" {
		p.Message = "intentional failure"
	}
	return fmt.Errorf("always_fail: %s", p.Message)
}
