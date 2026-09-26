package github

import (
	"errors"
	"testing"
)

func TestParsePRURL(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    PullRequestRef
		wantErr bool
	}{
		{"canonical", "https://github.com/company/project/pull/123", PullRequestRef{"company", "project", 123}, false},
		{"no scheme", "github.com/chirag/project/pull/42", PullRequestRef{"chirag", "project", 42}, false},
		{"www host", "https://www.github.com/o/r/pull/7", PullRequestRef{"o", "r", 7}, false},
		{"files tab", "https://github.com/o/r/pull/7/files", PullRequestRef{"o", "r", 7}, false},
		{"query and fragment", "https://github.com/o/r/pull/7?diff=split#discussion_r1", PullRequestRef{"o", "r", 7}, false},
		{"surrounding whitespace", "  https://github.com/o/r/pull/7\n", PullRequestRef{"o", "r", 7}, false},
		{"empty", "", PullRequestRef{}, true},
		{"not a url", "hello world", PullRequestRef{}, true},
		{"wrong host", "https://gitlab.com/o/r/pull/7", PullRequestRef{}, true},
		{"lookalike host", "https://github.com.evil.io/o/r/pull/7", PullRequestRef{}, true},
		{"issue not pull", "https://github.com/o/r/issues/7", PullRequestRef{}, true},
		{"missing number", "https://github.com/o/r/pull/", PullRequestRef{}, true},
		{"non numeric", "https://github.com/o/r/pull/abc", PullRequestRef{}, true},
		{"zero", "https://github.com/o/r/pull/0", PullRequestRef{}, true},
		{"negative", "https://github.com/o/r/pull/-3", PullRequestRef{}, true},
		{"ftp scheme", "ftp://github.com/o/r/pull/7", PullRequestRef{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParsePRURL(tt.input)
			if tt.wantErr {
				if !errors.Is(err, ErrInvalidPRURL) {
					t.Fatalf("error = %v, want ErrInvalidPRURL", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestPullRequestRefString(t *testing.T) {
	got := PullRequestRef{Owner: "o", Repo: "r", Number: 5}.String()
	if got != "o/r#5" {
		t.Fatalf("got %q, want o/r#5", got)
	}
}
