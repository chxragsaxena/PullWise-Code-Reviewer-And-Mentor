package embed

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeCohere answers /v2/embed with one 2-dim vector per text, whose first
// component is the text's global position, so ordering can be asserted.
func fakeCohere(t *testing.T, calls *[]cohereRequest) *CohereEmbedder {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization = %q, want Bearer test-key", got)
		}
		if r.URL.Path != "/v2/embed" {
			t.Errorf("path = %q, want /v2/embed", r.URL.Path)
		}
		var req cohereRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		*calls = append(*calls, req)

		vectors := make([][]float32, len(req.Texts))
		for i, text := range req.Texts {
			var n int
			fmt.Sscanf(text, "t%d", &n)
			vectors[i] = []float32{float32(n), 0.5}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "x", "embeddings": map[string]any{"float": vectors},
		})
	}))
	t.Cleanup(srv.Close)
	return NewCohereEmbedder("test-key", WithCohereBaseURL(srv.URL))
}

func texts(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("t%d", i)
	}
	return out
}

func TestEmbedBatchesAtTheCallLimitAndKeepsOrder(t *testing.T) {
	tests := []struct {
		name      string
		n         int
		wantCalls []int
	}{
		{"empty makes no call", 0, nil},
		{"one text", 1, []int{1}},
		{"exactly one full batch", 96, []int{96}},
		{"one over spills to a second call", 97, []int{96, 1}},
		{"several batches", 200, []int{96, 96, 8}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls []cohereRequest
			e := fakeCohere(t, &calls)

			got, err := e.Embed(context.Background(), texts(tt.n), Document)
			if err != nil {
				t.Fatalf("Embed: %v", err)
			}
			if len(got) != tt.n {
				t.Fatalf("got %d vectors, want %d", len(got), tt.n)
			}
			for i, v := range got {
				if int(v[0]) != i {
					t.Fatalf("vector %d belongs to text %d: order was not preserved", i, int(v[0]))
				}
			}
			var sizes []int
			for _, c := range calls {
				sizes = append(sizes, len(c.Texts))
			}
			if fmt.Sprint(sizes) != fmt.Sprint(tt.wantCalls) {
				t.Fatalf("call sizes = %v, want %v", sizes, tt.wantCalls)
			}
		})
	}
}

func TestEmbedUsesTheModelAndInputTypeForTheKind(t *testing.T) {
	tests := []struct {
		kind      Kind
		wantModel string
		wantType  string
	}{
		{Document, DefaultCohereDocumentModel, "search_document"},
		{Query, DefaultCohereQueryModel, "search_query"},
	}
	for _, tt := range tests {
		var calls []cohereRequest
		e := fakeCohere(t, &calls)
		if _, err := e.Embed(context.Background(), texts(1), tt.kind); err != nil {
			t.Fatalf("Embed: %v", err)
		}
		got := calls[0]
		if got.Model != tt.wantModel || got.InputType != tt.wantType {
			t.Errorf("kind %d: model=%q input_type=%q, want %q / %q", tt.kind, got.Model, got.InputType, tt.wantModel, tt.wantType)
		}
		if len(got.EmbeddingTypes) != 1 || got.EmbeddingTypes[0] != "float" {
			t.Errorf("embedding_types = %v, want [float]", got.EmbeddingTypes)
		}
	}
}

func TestEmbedModelsAreOverridable(t *testing.T) {
	var calls []cohereRequest
	e := fakeCohere(t, &calls)
	WithCohereModels("doc-model", "query-model")(e)

	_, _ = e.Embed(context.Background(), texts(1), Document)
	_, _ = e.Embed(context.Background(), texts(1), Query)
	if calls[0].Model != "doc-model" || calls[1].Model != "query-model" {
		t.Fatalf("models = %q, %q", calls[0].Model, calls[1].Model)
	}
}

func TestEmbedReturnsTheAPIErrorAndStopsBatching(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, `{"id":"e1","message":"too many requests"}`)
	}))
	t.Cleanup(srv.Close)
	e := NewCohereEmbedder("test-key", WithCohereBaseURL(srv.URL))

	_, err := e.Embed(context.Background(), texts(200), Document)

	var apiErr *CohereAPIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 429 || apiErr.Message != "too many requests" {
		t.Fatalf("error = %v, want a 429 CohereAPIError", err)
	}
	if hits != 1 {
		t.Fatalf("made %d calls after the first failure, want 1", hits)
	}
	if !strings.Contains(err.Error(), "texts 1-96 of 200") {
		t.Errorf("error should say which batch failed: %v", err)
	}
}

func TestEmbedRejectsAMismatchedVectorCount(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"embeddings":{"float":[[0.1]]}}`)
	}))
	t.Cleanup(srv.Close)
	e := NewCohereEmbedder("test-key", WithCohereBaseURL(srv.URL))

	if _, err := e.Embed(context.Background(), texts(3), Document); err == nil || !strings.Contains(err.Error(), "1 vectors for 3 texts") {
		t.Fatalf("error = %v, want a vector-count mismatch", err)
	}
}

func TestEmbedHonorsContextCancellation(t *testing.T) {
	var calls []cohereRequest
	e := fakeCohere(t, &calls)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := e.Embed(ctx, texts(1), Document); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}
