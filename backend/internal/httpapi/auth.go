package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/auth"
)

const (
	sessionCookieName = "pullwise_session"
	stateCookieName   = "pullwise_oauth_state"
	stateCookieMaxAge = 5 * 60 // 5 minutes: only needs to survive the redirect to GitHub and back
)

// AuthService is what the auth handlers need from the login use case.
type AuthService interface {
	Login() (redirectURL, state string, err error)
	Callback(ctx context.Context, code, gotState, wantState string) (sessionToken string, expiresAt time.Time, err error)
	CurrentUser(ctx context.Context, sessionToken string) (auth.User, error)
	Logout(ctx context.Context, sessionToken string) error
}

// handleLogin redirects the browser to GitHub, after setting a short-lived
// cookie holding the CSRF state value Callback will check for.
func (a *API) handleLogin(w http.ResponseWriter, r *http.Request) {
	if a.auth == nil {
		writeError(w, http.StatusServiceUnavailable, "auth_disabled", "GitHub login is not configured on this server.")
		return
	}
	redirectURL, state, err := a.auth.Login()
	if err != nil {
		slog.Error("start github login", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not start login.")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: stateCookieName, Value: state, Path: "/", MaxAge: stateCookieMaxAge,
		HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: a.secureCookies,
	})
	http.Redirect(w, r, redirectURL, http.StatusFound)
}

// handleCallback completes the login GitHub redirected back from.
func (a *API) handleCallback(w http.ResponseWriter, r *http.Request) {
	if a.auth == nil {
		notFound(w, r)
		return
	}

	// The state cookie is one-time use: clear it regardless of outcome.
	var wantState string
	if c, err := r.Cookie(stateCookieName); err == nil {
		wantState = c.Value
	}
	http.SetCookie(w, &http.Cookie{Name: stateCookieName, Value: "", Path: "/", MaxAge: -1})

	if ghErr := r.URL.Query().Get("error"); ghErr != "" {
		writeError(w, http.StatusBadRequest, "oauth_denied", "GitHub login was not completed: "+ghErr)
		return
	}
	code := r.URL.Query().Get("code")
	if code == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "Missing authorization code.")
		return
	}

	token, expiresAt, err := a.auth.Callback(r.Context(), code, r.URL.Query().Get("state"), wantState)
	if err != nil {
		if errors.Is(err, auth.ErrStateMismatch) {
			writeError(w, http.StatusBadRequest, "invalid_state", "Login request expired or was tampered with. Please try again.")
			return
		}
		slog.Error("github oauth callback", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Login failed. Please try again.")
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name: sessionCookieName, Value: token, Path: "/", Expires: expiresAt,
		HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: a.secureCookies,
	})
	http.Redirect(w, r, a.frontendURL, http.StatusFound)
}

// handleMe reports who is logged in, or {"user": null} if no one is — this
// is a status check, not a protected resource, so it always answers 200.
func (a *API) handleMe(w http.ResponseWriter, r *http.Request) {
	user := a.sessionUser(r)
	if user == nil {
		writeJSON(w, http.StatusOK, map[string]any{"user": nil})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": toUserDTO(*user)})
}

// handleLogout ends the current session, if any, and always clears the cookie.
func (a *API) handleLogout(w http.ResponseWriter, r *http.Request) {
	if a.auth != nil {
		if c, err := r.Cookie(sessionCookieName); err == nil {
			if err := a.auth.Logout(r.Context(), c.Value); err != nil {
				slog.Warn("logout", "error", err)
			}
		}
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookieName, Value: "", Path: "/", MaxAge: -1})
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// sessionUser resolves the session cookie to a user, or nil if there isn't
// one, it's invalid, or auth isn't configured. Handlers that want a user
// without requiring one (like handleMe) use this directly.
func (a *API) sessionUser(r *http.Request) *auth.User {
	if a.auth == nil {
		return nil
	}
	c, err := r.Cookie(sessionCookieName)
	if err != nil {
		return nil
	}
	user, err := a.auth.CurrentUser(r.Context(), c.Value)
	if err != nil {
		return nil
	}
	return &user
}

type userDTO struct {
	ID        string `json:"id"`
	Login     string `json:"login"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatarUrl"`
}

func toUserDTO(u auth.User) userDTO {
	return userDTO{ID: u.ID, Login: u.GitHubLogin, Name: u.Name, AvatarURL: u.AvatarURL}
}
