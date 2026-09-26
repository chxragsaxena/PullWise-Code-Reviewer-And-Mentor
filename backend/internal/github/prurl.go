// Package github talks to GitHub: parsing PR links and fetching pull requests.
package github

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// ErrInvalidPRURL is returned when a string is not a GitHub pull request link.
var ErrInvalidPRURL = errors.New("not a valid GitHub pull request URL")

// PullRequestRef identifies a pull request.
type PullRequestRef struct {
	Owner  string
	Repo   string
	Number int
}

// String returns the canonical form, e.g. "owner/repo#123".
func (r PullRequestRef) String() string {
	return fmt.Sprintf("%s/%s#%d", r.Owner, r.Repo, r.Number)
}

// ParsePRURL extracts owner, repo and number from a link such as
// https://github.com/owner/repo/pull/123. The scheme is optional, and trailing
// segments such as /files, query strings and fragments are ignored.
func ParsePRURL(raw string) (PullRequestRef, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return PullRequestRef{}, ErrInvalidPRURL
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}

	u, err := url.Parse(raw)
	if err != nil {
		return PullRequestRef{}, fmt.Errorf("%w: %v", ErrInvalidPRURL, err)
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return PullRequestRef{}, ErrInvalidPRURL
	}
	if host := strings.ToLower(u.Hostname()); host != "github.com" && host != "www.github.com" {
		return PullRequestRef{}, ErrInvalidPRURL
	}

	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 4 || parts[2] != "pull" || parts[0] == "" || parts[1] == "" {
		return PullRequestRef{}, ErrInvalidPRURL
	}
	number, err := strconv.Atoi(parts[3])
	if err != nil || number < 1 {
		return PullRequestRef{}, ErrInvalidPRURL
	}

	return PullRequestRef{Owner: parts[0], Repo: parts[1], Number: number}, nil
}
