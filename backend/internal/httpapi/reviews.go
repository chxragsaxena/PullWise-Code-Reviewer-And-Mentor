package httpapi

import (
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strconv"

	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/review"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/service"
)

// maxCreateBody bounds the POST /api/reviews body; a link is a few hundred bytes.
const maxCreateBody = 4 << 10

func (a *API) createReview(w http.ResponseWriter, r *http.Request) {
	if a.create != nil {
		if ok, wait := a.create.allow(clientKey(r)); !ok {
			w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
			writeError(w, http.StatusTooManyRequests, "rate_limited", "Too many reviews started. Please wait a moment and try again.")
			return
		}
	}

	var body struct {
		PRURL string `json:"prUrl"`
	}
	if apiErr := decodeJSON(w, r, maxCreateBody, &body); apiErr != nil {
		apiErr.write(w)
		return
	}

	rv, err := a.svc.Create(r.Context(), body.PRURL)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, toSummaryDTO(rv))
}

func (a *API) listReviews(w http.ResponseWriter, r *http.Request) {
	limit := 0 // the service applies its default
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "limit must be a whole number.")
			return
		}
		limit = n
	}

	reviews, err := a.svc.List(r.Context(), limit)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}

	out := make([]reviewSummaryDTO, 0, len(reviews))
	for _, rv := range reviews {
		out = append(out, toSummaryDTO(rv))
	}
	writeJSON(w, http.StatusOK, map[string]any{"reviews": out})
}

func (a *API) getReview(w http.ResponseWriter, r *http.Request) {
	rv, err := a.svc.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toReviewDTO(rv))
}

func (a *API) getFinding(w http.ResponseWriter, r *http.Request) {
	detail, err := a.svc.GetFinding(r.Context(), r.PathValue("id"), r.PathValue("findingId"))
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toFindingDetailDTO(detail))
}

// writeServiceError maps service errors to the documented status and code.
// Anything unexpected is logged in full but reported generically.
func writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, service.ErrInvalidPRURL):
		writeError(w, http.StatusBadRequest, "invalid_pr_url", "Not a valid GitHub pull request URL.")
	case errors.Is(err, review.ErrReviewNotFound):
		writeError(w, http.StatusNotFound, "review_not_found", "That review does not exist.")
	case errors.Is(err, review.ErrFindingNotFound):
		writeError(w, http.StatusNotFound, "finding_not_found", "That finding does not exist.")
	case errors.Is(err, service.ErrBusy):
		w.Header().Set("Retry-After", "30")
		writeError(w, http.StatusServiceUnavailable, "server_busy", "Too many reviews are in progress. Please try again shortly.")
	case errors.Is(err, service.ErrShuttingDown):
		writeError(w, http.StatusServiceUnavailable, "shutting_down", "The server is restarting. Please try again shortly.")
	default:
		slog.Error("request failed", "method", r.Method, "path", r.URL.Path, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Something went wrong.")
	}
}

// clientKey identifies the caller for rate limiting: the connection's address
// without the port. Forwarding headers are deliberately not trusted, because any
// client can forge them.
func clientKey(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
