// Package embed turns text into vectors for similarity search (the V4
// repository knowledge base). Providers sit behind Embedder so the choice of
// service stays reversible.
package embed

import "context"

// Kind says what a text will be used for. Many embedding models produce
// different vectors for documents being indexed and queries searching them.
type Kind int

const (
	// Document is content being stored for later retrieval.
	Document Kind = iota
	// Query is a search string matched against stored documents.
	Query
)

// Embedder converts texts to vectors, one per input and in the same order.
type Embedder interface {
	Embed(ctx context.Context, texts []string, kind Kind) ([][]float32, error)
}
