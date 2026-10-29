package github

import "context"

type tokenKey struct{}

func WithRequestToken(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, tokenKey{}, token)
}

// RequestToken returns the token set by WithRequestToken, or "".
func RequestToken(ctx context.Context) string {
	t, _ := ctx.Value(tokenKey{}).(string)
	return t
}
