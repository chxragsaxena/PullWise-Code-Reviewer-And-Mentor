package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// scope grants read/write access to the repositories the user can see,
// public and private — needed to review a private repo on the user's behalf.
const scope = "repo"

// OAuth drives the GitHub OAuth App login flow: the authorize redirect, the
// code-for-token exchange, and reading the logged-in user's profile.
type OAuth struct {
	clientID     string
	clientSecret string
	redirectURL  string
	httpClient   *http.Client

	// Endpoints, overridable for tests; real GitHub hosts by default.
	authorizeURL string
	tokenURL     string
	userURL      string
}

// Option configures an OAuth.
type Option func(*OAuth)

// WithHTTPClient replaces the underlying HTTP client.
func WithHTTPClient(c *http.Client) Option { return func(o *OAuth) { o.httpClient = c } }

// WithEndpoints overrides all three GitHub endpoints at once (used by tests
// to point everything at one httptest server).
func WithEndpoints(authorizeURL, tokenURL, userURL string) Option {
	return func(o *OAuth) { o.authorizeURL, o.tokenURL, o.userURL = authorizeURL, tokenURL, userURL }
}

// NewOAuth builds an OAuth client for one registered GitHub OAuth App.
func NewOAuth(clientID, clientSecret, redirectURL string, opts ...Option) *OAuth {
	o := &OAuth{
		clientID:     clientID,
		clientSecret: clientSecret,
		redirectURL:  redirectURL,
		httpClient:   &http.Client{Timeout: 15 * time.Second},
		authorizeURL: "https://github.com/login/oauth/authorize",
		tokenURL:     "https://github.com/login/oauth/access_token",
		userURL:      "https://api.github.com/user",
	}
	for _, opt := range opts {
		opt(o)
	}
	return o
}

// NewState returns a random CSRF state value for the authorize redirect. The
// caller stores it (e.g. in a short-lived cookie) and checks it against the
// callback's state parameter before trusting the callback at all.
func NewState() (string, error) {
	b := make([]byte, 24)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", fmt.Errorf("generate oauth state: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// AuthorizeURL returns the URL to send the user's browser to.
func (o *OAuth) AuthorizeURL(state string) string {
	q := url.Values{
		"client_id":    {o.clientID},
		"redirect_uri": {o.redirectURL},
		"scope":        {scope},
		"state":        {state},
	}
	return o.authorizeURL + "?" + q.Encode()
}

// Token is what GitHub returns from the code exchange. RefreshToken and
// ExpiresAt are zero unless the OAuth App has "Expire user access tokens" on.
type Token struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
}

// Exchange trades an authorization code (from the callback) for a token.
func (o *OAuth) Exchange(ctx context.Context, code string) (Token, error) {
	return o.requestToken(ctx, url.Values{
		"client_id":     {o.clientID},
		"client_secret": {o.clientSecret},
		"code":          {code},
		"redirect_uri":  {o.redirectURL},
	})
}

// Refresh trades a refresh token for a new access token. GitHub rotates the
// refresh token: the one passed in stops working, so the caller must save the
// returned one.
func (o *OAuth) Refresh(ctx context.Context, refreshToken string) (Token, error) {
	return o.requestToken(ctx, url.Values{
		"client_id":     {o.clientID},
		"client_secret": {o.clientSecret},
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
	})
}

// requestToken posts to GitHub's token endpoint, which reports failures as a
// 200 response with an "error" field.
func (o *OAuth) requestToken(ctx context.Context, form url.Values) (Token, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return Token{}, fmt.Errorf("build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := o.httpClient.Do(req)
	if err != nil {
		return Token{}, fmt.Errorf("request token: %w", err)
	}
	defer resp.Body.Close()

	var body struct {
		AccessToken      string `json:"access_token"`
		RefreshToken     string `json:"refresh_token"`
		ExpiresIn        int    `json:"expires_in"`
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return Token{}, fmt.Errorf("decode token response: %w", err)
	}
	if body.Error != "" {
		return Token{}, fmt.Errorf("github oauth error: %s: %s", body.Error, body.ErrorDescription)
	}
	if body.AccessToken == "" {
		return Token{}, errors.New("github oauth: response had no access token")
	}

	tok := Token{AccessToken: body.AccessToken, RefreshToken: body.RefreshToken}
	if body.ExpiresIn > 0 {
		tok.ExpiresAt = time.Now().Add(time.Duration(body.ExpiresIn) * time.Second)
	}
	return tok, nil
}

// Profile is the subset of the GitHub user's profile this app stores.
type Profile struct {
	ID        int64
	Login     string
	Name      string
	Email     string
	AvatarURL string
}

// User is a logged-in user, as the store persists and returns them. Defined
// here (not in internal/store) so internal/store's methods can return it
// directly without internal/auth importing internal/store back.
type User struct {
	ID          string
	GitHubLogin string
	Name        string
	AvatarURL   string
}

// Tokens is one user's encrypted GitHub OAuth tokens, as stored and
// retrieved. RefreshEnc and ExpiresAt are nil for a grant that does not
// expire (a classic OAuth App token).
type Tokens struct {
	AccessEnc  []byte
	RefreshEnc []byte
	ExpiresAt  *time.Time
}

// FetchProfile reads the token's owner profile from GitHub.
func (o *OAuth) FetchProfile(ctx context.Context, accessToken string) (Profile, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, o.userURL, nil)
	if err != nil {
		return Profile{}, fmt.Errorf("build profile request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := o.httpClient.Do(req)
	if err != nil {
		return Profile{}, fmt.Errorf("request profile: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Profile{}, fmt.Errorf("github profile: unexpected status %d", resp.StatusCode)
	}

	var body struct {
		ID        int64  `json:"id"`
		Login     string `json:"login"`
		Name      string `json:"name"`
		Email     string `json:"email"`
		AvatarURL string `json:"avatar_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return Profile{}, fmt.Errorf("decode profile: %w", err)
	}
	return Profile{ID: body.ID, Login: body.Login, Name: body.Name, Email: body.Email, AvatarURL: body.AvatarURL}, nil
}
