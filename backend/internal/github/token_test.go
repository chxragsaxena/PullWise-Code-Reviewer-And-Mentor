package github

import (
	"context"
	"fmt"
	"net/http"
	"testing"
)

func TestRequestTokenOverridesTheClientsOwnToken(t *testing.T) {
	tests := []struct {
		name       string
		clientTok  string
		requestTok string
		wantAuth   string
	}{
		{"user token beats server token", "server", "user", "Bearer user"},
		{"user token with no server token", "", "user", "Bearer user"},
		{"no user token keeps the server token", "server", "", "Bearer server"},
		{"neither sends nothing", "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotAuth string
			client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotAuth = r.Header.Get("Authorization")
				fmt.Fprint(w, `{}`)
			}), WithToken(tt.clientTok))

			ctx := WithRequestToken(context.Background(), tt.requestTok)
			if _, err := client.FetchPullRequest(ctx, testRef); err != nil {
				t.Fatal(err)
			}
			if gotAuth != tt.wantAuth {
				t.Fatalf("Authorization = %q, want %q", gotAuth, tt.wantAuth)
			}
		})
	}
}
