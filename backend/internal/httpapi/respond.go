package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strings"
)

// errorBody is the single error format every endpoint uses.
type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// writeJSON sends v as JSON with the given status.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("write response", "error", err)
	}
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorBody{Error: errorDetail{Code: code, Message: message}})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, maxBytes int64, v any) *apiError {
	if mt := strings.TrimSpace(strings.SplitN(r.Header.Get("Content-Type"), ";", 2)[0]); mt != "application/json" {
		return &apiError{http.StatusUnsupportedMediaType, "unsupported_media_type", "Send the request as application/json."}
	}

	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBytes))
	if err := dec.Decode(v); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return &apiError{http.StatusRequestEntityTooLarge, "request_too_large", "The request body is too large."}
		}
		return &apiError{http.StatusBadRequest, "invalid_request", "The request body is not valid JSON."}
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return &apiError{http.StatusBadRequest, "invalid_request", "The request body must contain a single JSON value."}
	}
	return nil
}

// apiError is an error with the HTTP status and code to report it under.
type apiError struct {
	status  int
	code    string
	message string
}

func (e *apiError) write(w http.ResponseWriter) { writeError(w, e.status, e.code, e.message) }

type methods map[string]http.HandlerFunc

func (m methods) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h, ok := m[r.Method]; ok {
		h(w, r)
		return
	}
	allowed := make([]string, 0, len(m))
	for method := range m {
		allowed = append(allowed, method)
	}
	sort.Strings(allowed)
	w.Header().Set("Allow", strings.Join(allowed, ", "))
	writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "That method is not allowed for this endpoint.")
}

func notFound(w http.ResponseWriter, _ *http.Request) {
	writeError(w, http.StatusNotFound, "not_found", "No such endpoint.")
}
