package auth

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// sessionTTL bounds a session's lifetime regardless of the underlying GitHub
// token's own expiry — a classic (non-expiring) OAuth grant should still not
// mean a session that never requires logging in again.
const sessionTTL = 30 * 24 * time.Hour

// ErrStateMismatch means the callback's state parameter did not match the
// one issued for this login attempt — the request is not trusted.
var ErrStateMismatch = errors.New("oauth state mismatch")

// ErrTokenExpired means the stored GitHub access token has expired and could not
// be renewed; the user must sign in again to restore private-repo access.
var ErrTokenExpired = errors.New("github access token expired")

// Store is what Service needs from persistence. *store.Store satisfies this
// directly: its methods already return this package's User/Tokens types.
type Store interface {
	UpsertGitHubUser(ctx context.Context, p Profile) (User, error)
	SaveTokens(ctx context.Context, userID string, accessEnc, refreshEnc []byte, expiresAt *time.Time) error
	GetTokens(ctx context.Context, userID string) (Tokens, error)
	CreateSession(ctx context.Context, hash SessionTokenHash, userID string, expiresAt time.Time) error
	GetSessionUser(ctx context.Context, hash SessionTokenHash) (User, error)
	DeleteSession(ctx context.Context, hash SessionTokenHash) error
}

// Service is the GitHub login use case: starting a login, completing it,
// reading who a session belongs to, and logging out.
type Service struct {
	oauth *OAuth
	crypt *Encryptor
	store Store

	// refreshMu serializes token refreshes. This assumes one server instance;
	// with several, a row lock in the database would be needed instead.
	refreshMu sync.Mutex
}

// NewService composes the login flow's pieces.
func NewService(oauth *OAuth, crypt *Encryptor, store Store) *Service {
	return &Service{oauth: oauth, crypt: crypt, store: store}
}

// Login starts a login attempt: a URL to send the browser to, and the state
// value the caller must keep (e.g. in a short-lived cookie) and pass back to
// Callback unchanged.
func (s *Service) Login() (redirectURL, state string, err error) {
	state, err = NewState()
	if err != nil {
		return "", "", err
	}
	return s.oauth.AuthorizeURL(state), state, nil
}

// Callback completes a login: it requires gotState to equal wantState
// (the CSRF check — do this before anything else), exchanges the code,
// reads the GitHub profile, stores the user and their encrypted tokens, and
// starts a new session.
//
// Returns the raw session token (for the caller to set as a cookie) and when
// it expires.
func (s *Service) Callback(ctx context.Context, code, gotState, wantState string) (sessionToken string, expiresAt time.Time, err error) {
	if wantState == "" || gotState != wantState {
		return "", time.Time{}, ErrStateMismatch
	}

	tok, err := s.oauth.Exchange(ctx, code)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("exchange code: %w", err)
	}
	profile, err := s.oauth.FetchProfile(ctx, tok.AccessToken)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("fetch profile: %w", err)
	}
	user, err := s.store.UpsertGitHubUser(ctx, profile)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("save user: %w", err)
	}

	accessEnc, err := s.crypt.Encrypt(tok.AccessToken)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("encrypt access token: %w", err)
	}
	var refreshEnc []byte
	var githubExpiresAt *time.Time
	if tok.RefreshToken != "" {
		if refreshEnc, err = s.crypt.Encrypt(tok.RefreshToken); err != nil {
			return "", time.Time{}, fmt.Errorf("encrypt refresh token: %w", err)
		}
		githubExpiresAt = &tok.ExpiresAt
	}
	if err := s.store.SaveTokens(ctx, user.ID, accessEnc, refreshEnc, githubExpiresAt); err != nil {
		return "", time.Time{}, fmt.Errorf("save tokens: %w", err)
	}

	sessionToken, hash, err := NewSessionToken()
	if err != nil {
		return "", time.Time{}, err
	}
	sessionExpiresAt := time.Now().Add(sessionTTL)
	if err := s.store.CreateSession(ctx, hash, user.ID, sessionExpiresAt); err != nil {
		return "", time.Time{}, fmt.Errorf("create session: %w", err)
	}
	return sessionToken, sessionExpiresAt, nil
}

// CurrentUser resolves a session cookie's value to the logged-in user.
func (s *Service) CurrentUser(ctx context.Context, sessionToken string) (User, error) {
	return s.store.GetSessionUser(ctx, HashSessionToken(sessionToken))
}

// Logout ends a session.
func (s *Service) Logout(ctx context.Context, sessionToken string) error {
	return s.store.DeleteSession(ctx, HashSessionToken(sessionToken))
}

// refreshSkew refreshes a little before the stated expiry, so a token never
// expires between being handed out and being used.
const refreshSkew = time.Minute

// UserGitHubToken returns a user's GitHub access token, for making API calls
// on their behalf (e.g. reviewing a private repository). An expired token is
// refreshed with the stored refresh token and the new pair saved.
//
// It returns ErrTokenExpired only when the token cannot be renewed (no refresh
// token, or GitHub refused it), so callers fall back to anonymous access
// rather than sending a token GitHub will reject even for public repositories.
func (s *Service) UserGitHubToken(ctx context.Context, userID string) (string, error) {
	tokens, err := s.store.GetTokens(ctx, userID)
	if err != nil {
		return "", fmt.Errorf("get tokens: %w", err)
	}
	if len(tokens.AccessEnc) == 0 {
		return "", errors.New("user has no stored github token")
	}
	if !needsRefresh(tokens) {
		return s.crypt.Decrypt(tokens.AccessEnc)
	}

	// Refresh tokens are single-use, so two concurrent refreshes would break
	// one of them. Serialize, and re-read: another request may have just done it.
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()
	tokens, err = s.store.GetTokens(ctx, userID)
	if err != nil {
		return "", fmt.Errorf("get tokens: %w", err)
	}
	if !needsRefresh(tokens) {
		return s.crypt.Decrypt(tokens.AccessEnc)
	}
	return s.refresh(ctx, userID, tokens)
}

func needsRefresh(t Tokens) bool {
	return t.ExpiresAt != nil && !t.ExpiresAt.After(time.Now().Add(refreshSkew))
}

func (s *Service) refresh(ctx context.Context, userID string, tokens Tokens) (string, error) {
	if len(tokens.RefreshEnc) == 0 {
		return "", ErrTokenExpired
	}
	refreshToken, err := s.crypt.Decrypt(tokens.RefreshEnc)
	if err != nil {
		return "", fmt.Errorf("decrypt refresh token: %w", err)
	}

	fresh, err := s.oauth.Refresh(ctx, refreshToken)
	if err != nil {
		// Expired or revoked refresh token: the user must sign in again.
		return "", fmt.Errorf("%w: %v", ErrTokenExpired, err)
	}

	accessEnc, err := s.crypt.Encrypt(fresh.AccessToken)
	if err != nil {
		return "", fmt.Errorf("encrypt access token: %w", err)
	}
	// GitHub rotates the refresh token; if none came back, keep the old one.
	refreshEnc := tokens.RefreshEnc
	if fresh.RefreshToken != "" {
		if refreshEnc, err = s.crypt.Encrypt(fresh.RefreshToken); err != nil {
			return "", fmt.Errorf("encrypt refresh token: %w", err)
		}
	}
	var expiresAt *time.Time
	if !fresh.ExpiresAt.IsZero() {
		expiresAt = &fresh.ExpiresAt
	}
	if err := s.store.SaveTokens(ctx, userID, accessEnc, refreshEnc, expiresAt); err != nil {
		return "", fmt.Errorf("save refreshed tokens: %w", err)
	}
	return fresh.AccessToken, nil
}
