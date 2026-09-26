package httpapi

import (
	"log/slog"
	"net/http"
	"time"
)

// recoverer turns a panic in a handler into a 500 response instead of a dropped
// connection, and logs it.
func recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("panic in handler", "panic", rec, "method", r.Method, "path", r.URL.Path)
				writeError(w, http.StatusInternalServerError, "internal_error", "something went wrong")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// statusRecorder captures the status code written by a handler.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// Unwrap exposes the underlying writer so http.ResponseController can flush it.
// Without this, wrapping the writer would silently break streaming responses.
func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

// secureHeaders sets defensive headers on every response. Responses are
// per-request and change over time, so they must not be cached.
func secureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

// requestLogger logs one line per request.
func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		slog.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"duration", time.Since(start),
		)
	})
}
