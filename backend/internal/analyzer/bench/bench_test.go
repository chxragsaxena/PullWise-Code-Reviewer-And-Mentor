// Package bench measures how fast each analyzer reads a large real source file.
//
//	BENCH_GO=path/to/file.go BENCH_TS=path/to/file.ts BENCH_PY=path/to/file.py \
//	  go test -run xxx -bench . -benchmem ./internal/analyzer/bench/
package bench

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/analyzer/goast"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/analyzer/pyast"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/analyzer/tsast"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/lang"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/review"
)

func benchFile(b *testing.B, env, language string, analyzer review.StaticAnalyzer) {
	path := os.Getenv(env)
	if path == "" {
		b.Skipf("set %s to a source file", env)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		b.Fatal(err)
	}
	lines := strings.Count(string(data), "\n") + 1
	input := review.AnalysisInput{Files: []review.ChangedFile{{Path: path, Language: language, Content: string(data)}}}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := analyzer.Analyze(context.Background(), input); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	b.ReportMetric(float64(lines)*float64(b.N)/b.Elapsed().Seconds(), "lines/s")
	b.ReportMetric(float64(lines), "lines/file")
}

func BenchmarkGoAnalyzer(b *testing.B) { benchFile(b, "BENCH_GO", lang.Go, goast.New(goast.Config{})) }
func BenchmarkTypeScriptAnalyzer(b *testing.B) {
	benchFile(b, "BENCH_TS", lang.TypeScript, tsast.New(tsast.Config{}))
}
func BenchmarkPythonAnalyzer(b *testing.B) {
	benchFile(b, "BENCH_PY", lang.Python, pyast.New(pyast.Config{}))
}
