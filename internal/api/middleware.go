package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/google/uuid"

	"codity.ai/scheduler/internal/domain"
	"codity.ai/scheduler/pkg/auth"
)

type ctxKey int

const (
	ctxKeyClaims ctxKey = iota
	ctxKeyRequestID
)

func claimsFromCtx(ctx context.Context) *domain.Claims {
	c, _ := ctx.Value(ctxKeyClaims).(*domain.Claims)
	return c
}

func requestIDFromCtx(ctx context.Context) string {
	s, _ := ctx.Value(ctxKeyRequestID).(string)
	return s
}

// RequestID injects a unique request ID into the context and response header.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			id = uuid.New().String()
		}
		w.Header().Set("X-Request-ID", id)
		ctx := context.WithValue(r.Context(), ctxKeyRequestID, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequestLogger logs method, path, status, and duration for every request.
func RequestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(sw, r)
			logger.Info("http request",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", sw.status),
				slog.Duration("duration", time.Since(start)),
				slog.String("request_id", requestIDFromCtx(r.Context())),
			)
		})
	}
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

// PanicRecovery catches panics in downstream handlers, logs the stack, and
// returns 500 instead of crashing the process.
func PanicRecovery(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					logger.Error("panic recovered",
						slog.Any("error", rec),
						slog.String("stack", string(debug.Stack())),
						slog.String("request_id", requestIDFromCtx(r.Context())),
					)
					writeError(w, http.StatusInternalServerError, "INTERNAL", "internal server error", nil)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// Auth validates the JWT Bearer token, extracts claims, and stores them in context.
func Auth(jwt *auth.JWTService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			if header == "" {
				writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing authorization header", nil)
				return
			}
			token, found := strings.CutPrefix(header, "Bearer ")
			if !found {
				writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid authorization format", nil)
				return
			}

			claims, err := jwt.ValidateToken(token)
			if err != nil {
				writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", fmt.Sprintf("invalid token: %s", err), nil)
				return
			}

			ctx := context.WithValue(r.Context(), ctxKeyClaims, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
