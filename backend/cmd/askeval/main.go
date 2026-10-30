// Command askeval measures how often Ask's retrieval brings back the file that answers a question.
//
// It needs a running knowledge service and a Groq key; the first run indexes each repository.
// Usage: go run ./cmd/askeval [-k 6] [-cases cmd/askeval/cases.json]
package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/ai"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/ask"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/config"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/github"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/knowledge"
)

//go:embed cases.json
var defaultCases []byte

type testCase struct {
	Repo     string   `json:"repo"`
	Question string   `json:"question"`
	Expect   []string `json:"expect"`
}

// mode is one way of searching, with the switches that produce it.
type mode struct {
	name       string
	vectorOnly bool
	noRewrite  bool
}

var modes = []mode{
	{name: "vectors only", vectorOnly: true},
	{name: "vectors + keywords", noRewrite: true},
	{name: "vectors + keywords + rewrite"},
}

// result is how one mode did on one question: the rank (1-based) of the first expected file, or 0.
type result struct{ rank int }

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "askeval:", err)
		os.Exit(1)
	}
}

func run() error {
	k := flag.Int("k", 6, "excerpts the AI is given")
	casesFile := flag.String("cases", "", "JSON file of cases (default: the built-in set)")
	flag.Parse()

	if _, err := config.LoadDotEnv(".env", "../.env"); err != nil {
		return fmt.Errorf("load .env: %w", err)
	}
	cases, err := loadCases(*casesFile)
	if err != nil {
		return err
	}

	kb, chat, repos, err := connect()
	if err != nil {
		return err
	}

	ctx := context.Background()
	results := make([][]result, len(modes))
	for mi, m := range modes {
		svc := ask.New(ask.Config{Knowledge: kb, Repos: repos, Chat: chat, VectorOnly: m.vectorOnly, NoRewrite: m.noRewrite})
		for ci, c := range cases {
			rctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
			excerpts, err := svc.Retrieve(rctx, ask.Request{Repo: c.Repo, Question: c.Question})
			cancel()
			if err != nil {
				return fmt.Errorf("%s / %q: %w", m.name, c.Question, err)
			}
			results[mi] = append(results[mi], result{rank: rankOf(excerpts, c.Expect, *k)})
			fmt.Fprintf(os.Stderr, "  [%s] %d/%d\r", m.name, ci+1, len(cases))
		}
		fmt.Fprintln(os.Stderr)
	}
	report(cases, results, *k)
	return nil
}

func loadCases(path string) ([]testCase, error) {
	data := defaultCases
	if path != "" {
		var err error
		if data, err = os.ReadFile(path); err != nil {
			return nil, fmt.Errorf("read cases: %w", err)
		}
	}
	var cases []testCase
	if err := json.Unmarshal(data, &cases); err != nil {
		return nil, fmt.Errorf("parse cases: %w", err)
	}
	if len(cases) == 0 {
		return nil, fmt.Errorf("no cases")
	}
	return cases, nil
}

func connect() (*knowledge.Client, ai.Chatter, *github.Client, error) {
	url, secret, groqKey := os.Getenv("KNOWLEDGE_URL"), os.Getenv("KNOWLEDGE_JWT_SECRET"), os.Getenv("GROQ_API_KEY")
	if url == "" || secret == "" || groqKey == "" {
		return nil, nil, nil, fmt.Errorf("set KNOWLEDGE_URL, KNOWLEDGE_JWT_SECRET and GROQ_API_KEY")
	}
	kb, err := knowledge.New(url, []byte(secret), "pullwise", "pullwise-knowledge")
	if err != nil {
		return nil, nil, nil, err
	}
	var opts []github.Option
	if t := os.Getenv("GITHUB_TOKEN"); t != "" {
		opts = append(opts, github.WithToken(t))
	}
	return kb, ai.NewGroqProvider(groqKey), github.NewClient(opts...), nil
}

// rankOf returns the 1-based position of the first excerpt from an expected file within the first k, or 0.
func rankOf(excerpts []knowledge.Snippet, expect []string, k int) int {
	for i, e := range excerpts {
		if i >= k {
			break
		}
		for _, want := range expect {
			if e.Path == want {
				return i + 1
			}
		}
	}
	return 0
}

func report(cases []testCase, results [][]result, k int) {
	fmt.Printf("\nRetrieval over %d questions, top %d excerpts (a hit = an expected file is among them)\n\n", len(cases), k)
	fmt.Printf("%-30s %8s %8s\n", "mode", "recall@"+fmt.Sprint(k), "MRR")
	for mi, m := range modes {
		hits, rr := 0, 0.0
		for _, r := range results[mi] {
			if r.rank > 0 {
				hits++
				rr += 1 / float64(r.rank)
			}
		}
		fmt.Printf("%-30s %7.0f%% %8.2f\n", m.name, 100*float64(hits)/float64(len(cases)), rr/float64(len(cases)))
	}

	fmt.Printf("\nPer question (rank of the first expected file, - = missed):\n")
	for ci, c := range cases {
		cells := make([]string, len(modes))
		for mi := range modes {
			if r := results[mi][ci].rank; r > 0 {
				cells[mi] = fmt.Sprint(r)
			} else {
				cells[mi] = "-"
			}
		}
		fmt.Printf("  %-4s %-4s %-4s  %s\n", cells[0], cells[1], cells[2], c.Question)
	}
	fmt.Println(strings.Repeat("-", 40))
	fmt.Println("columns: " + strings.Join(modeNames(), " | "))
}

func modeNames() []string {
	out := make([]string, len(modes))
	for i, m := range modes {
		out[i] = m.name
	}
	return out
}
