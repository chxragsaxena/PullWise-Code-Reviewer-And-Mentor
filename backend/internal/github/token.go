package github

import "context"

type tokenKey struct{}

// WithRequestToken returns a context whose GitHub calls authenticate with the
// given token instead of the client's own, e.g. a signed-in user's access
// token. It is a credential: never log it. An empty token changes nothing.
func WithRequestToken(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, tokenKey{}, token)
}

// RequestToken returns the token set by WithRequestToken, or "".
func RequestToken(ctx context.Context) string {
	t, _ := ctx.Value(tokenKey{}).(string)
	return t
}
