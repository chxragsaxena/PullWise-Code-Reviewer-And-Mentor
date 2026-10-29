package review

import "context"

type viewerKey struct{}

// WithViewer returns a context carrying the signed-in user; an empty id means anonymous.
func WithViewer(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, viewerKey{}, userID)
}

// ViewerFrom returns the signed-in user's id, or "" for an anonymous caller.
func ViewerFrom(ctx context.Context) string {
	id, _ := ctx.Value(viewerKey{}).(string)
	return id
}
