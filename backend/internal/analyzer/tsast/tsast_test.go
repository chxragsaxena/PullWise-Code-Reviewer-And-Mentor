package tsast

import (
	"context"
	"strings"
	"testing"

	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/lang"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/review"
)

func analyze(t *testing.T, cfg Config, path, content string) []review.Finding {
	t.Helper()
	if path == "" {
		path = "main.ts"
	}
	findings, err := New(cfg).Analyze(context.Background(), review.AnalysisInput{
		Files: []review.ChangedFile{{Path: path, Language: lang.TypeScript, Content: content}},
	})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	return findings
}

func checkIDs(findings []review.Finding) []string {
	ids := make([]string, len(findings))
	for i, f := range findings {
		ids[i] = f.CheckID
	}
	return ids
}

func contains(s, substr string) bool { return strings.Contains(s, substr) }

func TestLooseEquality(t *testing.T) {
	const src = `
function good1(a: number, b: number) { return a === b; }
function good2(a: number, b: number) { return a !== b; }
function bad1(a: number, b: number) { return a == b; }
function bad2(a: number, b: number) { return a != b; }
`
	findings := analyze(t, Config{}, "", src)
	var lines []int
	for _, f := range findings {
		if f.CheckID != "loose-equality" {
			t.Fatalf("unexpected check: %+v", f)
		}
		lines = append(lines, f.Line)
	}
	if len(findings) != 2 {
		t.Fatalf("findings = %v, want exactly bad1 and bad2 flagged", checkIDs(findings))
	}
	if lines[0] != 4 || lines[1] != 5 {
		t.Fatalf("lines = %v, want [4, 5]", lines)
	}
	if !contains(findings[0].Improvement, "===") {
		t.Errorf("improvement = %q, want it to suggest ===", findings[0].Improvement)
	}
	if !contains(findings[1].Improvement, "!==") {
		t.Errorf("improvement = %q, want it to suggest !==", findings[1].Improvement)
	}
}

func TestAnyType(t *testing.T) {
	const src = `
function good(x: number): string { return String(x); }
function badParam(x: any): string { return String(x); }
function badReturn(x: number): any { return x; }
`
	findings := analyze(t, Config{}, "", src)
	if len(findings) != 2 {
		t.Fatalf("findings = %v", findings)
	}
	for _, f := range findings {
		if f.CheckID != "any-type" {
			t.Fatalf("unexpected check: %+v", f)
		}
	}
	if !contains(findings[0].Found, "parameter") {
		t.Errorf("found = %q, want it to identify the parameter", findings[0].Found)
	}
	if !contains(findings[1].Found, "return type") {
		t.Errorf("found = %q, want it to identify the return type", findings[1].Found)
	}
}

func TestConsoleLog(t *testing.T) {
	const src = `
function f() {
  console.error("kept: a real error");
  console.warn("kept: a real warning");
  console.log("flagged: looks like debug output");
}
`
	findings := analyze(t, Config{}, "", src)
	if len(findings) != 1 || findings[0].CheckID != "console-log-left-in" || findings[0].Line != 5 {
		t.Fatalf("findings = %v, want exactly one console-log-left-in at line 5", findings)
	}
}

func TestConsoleLogIgnoresUnrelatedLogMethods(t *testing.T) {
	const src = `
const logger = { log: (s: string) => {} };
logger.log("not console, must not be flagged");
`
	if findings := analyze(t, Config{}, "", src); len(findings) != 0 {
		t.Fatalf("a non-console object's .log() must not be flagged: %v", findings)
	}
}

func TestUnhandledPromise(t *testing.T) {
	// loadData() stands in for any promise-returning call, so this test
	// isolates .then()/.catch() handling from the unrelated fetch-signal check.
	const src = `
loadData().then(r => use(r));
loadData().then(r => use(r)).catch(e => {});
const p = loadData().then(r => use(r));
f();
`
	findings := analyze(t, Config{}, "", src)
	if len(findings) != 1 || findings[0].CheckID != "unhandled-promise" || findings[0].Line != 2 {
		t.Fatalf("findings = %v, want exactly one unhandled-promise at line 2", findings)
	}
}

func TestFetchWithoutAbortSignal(t *testing.T) {
	const src = `
fetch(url);
fetch(url, { signal: ctrl.signal });
fetch(url, { headers: { "X-Test": "1" } });
fetch(url, opts);
`
	findings := analyze(t, Config{}, "", src)
	if len(findings) != 2 {
		t.Fatalf("findings = %v, want lines 2 and 4 flagged (bare call, and an object with no signal)", findings)
	}
	if findings[0].Line != 2 || findings[1].Line != 4 {
		t.Fatalf("lines = [%d, %d], want [2, 4]", findings[0].Line, findings[1].Line)
	}
	for _, f := range findings {
		if f.CheckID != "fetch-without-abort-signal" {
			t.Fatalf("unexpected check: %+v", f)
		}
	}
}

func TestFetchWithVariableOptionsIsNotFlagged(t *testing.T) {
	// opts might contain a signal; we can't tell without evaluating it, so we
	// deliberately don't guess (favors fewer false positives).
	const src = `fetch(url, opts);`
	if findings := analyze(t, Config{}, "", src); len(findings) != 0 {
		t.Fatalf("a non-literal options argument must not be flagged: %v", findings)
	}
}

func TestHighComplexity(t *testing.T) {
	const src = `
function simple(x: number): number {
  if (x > 0) { return 1; }
  return 0;
}
`
	if findings := analyze(t, Config{ComplexityThreshold: 5}, "", src); len(findings) != 0 {
		t.Fatalf("complexity 2 must not be flagged at threshold 5: %v", findings)
	}

	findings := analyze(t, Config{ComplexityThreshold: 1}, "", src)
	if len(findings) != 1 || findings[0].CheckID != "high-complexity" {
		t.Fatalf("findings = %v, want one high-complexity at threshold 1", findings)
	}
	if !contains(findings[0].Found, "simple") {
		t.Errorf("found = %q, want it to name the function", findings[0].Found)
	}
}

func TestHighComplexityCountsBranchesAndLogicalOperators(t *testing.T) {
	const src = `
function f(a: boolean, b: boolean, c: boolean): number {
  switch (true) {
    case a && b:
      return 1;
    case b || c:
      return 2;
  }
  for (let i = 0; i < 10; i++) {
    if (i === 5) { continue; }
  }
  return 0;
}
`
	// base 1 + 2 switch cases + 2 (&&,||) + 1 for + 1 if = 7
	findings := analyze(t, Config{ComplexityThreshold: 6}, "", src)
	if len(findings) != 1 || !contains(findings[0].Evidence, "is 7") {
		t.Fatalf("findings = %v, want one at threshold 6 with complexity 7", findings)
	}
	if findings := analyze(t, Config{ComplexityThreshold: 7}, "", src); len(findings) != 0 {
		t.Fatalf("complexity 7 must not be flagged at threshold 7 (only strictly greater): %v", findings)
	}
}

func TestTSXFileParsesWithJSX(t *testing.T) {
	const src = `
function Greeting(name: any) {
  return <div>Hello, {name}</div>;
}
`
	findings := analyze(t, Config{}, "main.tsx", src)
	if len(findings) != 1 || findings[0].CheckID != "any-type" {
		t.Fatalf("findings = %v, want the any-type check to still work in a .tsx file", findings)
	}
}

func TestFileThatFailsToParseIsSkippedNotFatal(t *testing.T) {
	findings, err := New(Config{}).Analyze(context.Background(), review.AnalysisInput{
		Files: []review.ChangedFile{
			// tree-sitter is error-tolerant, so force a real parser failure
			// with a context so short the parse can't finish.
			{Path: "fine.ts", Language: lang.TypeScript, Content: "const x = 1 == 2;\n"},
		},
	})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if len(findings) != 1 || findings[0].File != "fine.ts" {
		t.Fatalf("findings = %v", findings)
	}
}

func TestNonTSAndEmptyFilesAreSkipped(t *testing.T) {
	findings, err := New(Config{}).Analyze(context.Background(), review.AnalysisInput{
		Files: []review.ChangedFile{
			{Path: "app.go", Language: lang.Go, Content: "package main"},
			{Path: "empty.ts", Language: lang.TypeScript, Content: ""},
			{Path: "whitespace.ts", Language: lang.TypeScript, Content: "   \n\t\n"},
		},
	})
	if err != nil || len(findings) != 0 {
		t.Fatalf("findings = %v, err = %v; want none and no panic", findings, err)
	}
}

func TestAnalyzeStopsOnCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := New(Config{}).Analyze(ctx, review.AnalysisInput{
		Files: []review.ChangedFile{{Path: "a.ts", Language: lang.TypeScript, Content: "const x = 1;\n"}},
	})
	if err != context.Canceled {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

func TestFindingsLeaveSeverityAndSourceForTheServiceToFill(t *testing.T) {
	findings := analyze(t, Config{}, "", "const x: any = 1;\n")
	if len(findings) != 1 {
		t.Fatalf("findings = %v", findings)
	}
	f := findings[0]
	if f.Severity != "" || f.Source != "" || f.Confidence != 0 || f.Lesson != "" {
		t.Fatalf("analyzer must not set severity/source/confidence/lesson (the service fills those from the skill): %+v", f)
	}
	if f.SkillID != "typescript" {
		t.Fatalf("SkillID = %q, want typescript", f.SkillID)
	}
}
