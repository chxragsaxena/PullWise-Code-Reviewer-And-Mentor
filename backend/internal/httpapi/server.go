// Package httpapi exposes the REST API over net/http.
package httpapi

import "net/http"

// NewHandler builds the API router with its middleware.
func NewHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", handleHealth)

	// recoverer is outermost so a panic in any handler or middleware is caught.
	return recoverer(requestLogger(mux))
}

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
