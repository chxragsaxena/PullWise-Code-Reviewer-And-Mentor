package goast

import (
	"context"
	"testing"

	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/lang"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/review"
)

func analyze(t *testing.T, cfg Config, content string) []review.Finding {
	t.Helper()
	findings, err := New(cfg).Analyze(context.Background(), review.AnalysisInput{
		Files: []review.ChangedFile{{Path: "main.go", Language: lang.Go, Content: content}},
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

func TestIgnoredError(t *testing.T) {
	const src = `package p

import "os"

func good1() {
	_, err := os.ReadFile("a") // error kept: not flagged
	_ = err
}

func good2(m map[string]int) {
	_, ok := m["k"] // map index, not a call: not flagged
	_ = ok
}

func good3(x any) {
	_, ok := x.(int) // type assertion, not a call: not flagged
	_ = ok
}

func good4() {
	a, _ := 1, 2 // two RHS values, not one call's results: not flagged
	_ = a
}

func bad1() {
	data, _ := os.ReadFile("a")
	_ = data
}

func bad2() {
	var data []byte
	data, _ = os.ReadFile("a") // plain assignment, not just :=
	_ = data
}
`
	findings := analyze(t, Config{}, src)
	var lines []int
	for _, f := range findings {
		if f.CheckID != "ignored-error" {
			t.Fatalf("unexpected check: %+v", f)
		}
		lines = append(lines, f.Line)
	}
	if len(findings) != 2 {
		t.Fatalf("findings = %v, want exactly bad1 and bad2 flagged", checkIDs(findings))
	}
	if lines[0] != 26 || lines[1] != 32 {
		t.Fatalf("lines = %v, want [26, 32]", lines)
	}
	f := findings[0]
	if f.File != "main.go" || f.Evidence == "" || f.Found == "" || f.Why == "" || f.Improvement == "" || f.SkillID != "go" {
		t.Fatalf("finding not fully populated: %+v", f)
	}
}

func TestContextNotFirst(t *testing.T) {
	const src = `package p

import "context"

func Fetch(id int, ctx context.Context) error { return nil }

func Good(ctx context.Context, id int) error { return nil }

func NoContext(id int) error { return nil }

type T struct{}

func (t *T) M(id int, ctx context.Context) error { return nil }

func (t T) N(ctx context.Context) error { return nil }
`
	findings := analyze(t, Config{}, src)
	if len(findings) != 2 {
		t.Fatalf("findings = %d (%v), want 2", len(findings), findings)
	}
	if findings[0].CheckID != "context-not-first" || findings[0].Line != 5 {
		t.Errorf("first finding = %+v", findings[0])
	}
	if got := findings[0].Found; got == "" || !contains(got, "Fetch") || !contains(got, "parameter 2") {
		t.Errorf("Fetch message = %q", got)
	}
	if got := findings[1].Found; !contains(got, "(*T).M") {
		t.Errorf("method message = %q, want it to name the receiver", got)
	}
}

func TestContextNotFirstIgnoresPackagesNamedContextThatArentTheStdlibOne(t *testing.T) {
	const src = `package p

import ctx "example.com/context"

func F(id int, c ctx.Context) error { return nil }
`
	if findings := analyze(t, Config{}, src); len(findings) != 0 {
		t.Fatalf("a non-stdlib context package must not be flagged: %v", findings)
	}
}

func TestHTTPCallWithoutContext(t *testing.T) {
	const src = `package p

import "net/http"

func f() {
	http.Get("http://x")
	http.Post("http://x", "text/plain", nil)
	http.Head("http://x")
	http.PostForm("http://x", nil)
	req, _ := http.NewRequest("GET", "http://x", nil)
	_ = req
}
`
	findings := analyze(t, Config{}, src)
	if len(findings) != 5 { // 4 http.* calls + 1 ignored-error from NewRequest's "_"
		t.Fatalf("findings = %d, want 5: %v", len(findings), findings)
	}
	byCheck := map[string]int{}
	for _, f := range findings {
		byCheck[f.CheckID]++
	}
	if byCheck["http-call-without-context"] != 4 || byCheck["ignored-error"] != 1 {
		t.Fatalf("counts = %v", byCheck)
	}
}

func TestHTTPCallWithoutContextFollowsAnImportAlias(t *testing.T) {
	const src = `package p

import httpclient "net/http"

func f() { httpclient.Get("http://x") }
`
	findings := analyze(t, Config{}, src)
	if len(findings) != 1 || findings[0].CheckID != "http-call-without-context" {
		t.Fatalf("findings = %v, want one http-call-without-context via the alias", findings)
	}
}

func TestHTTPPackageNotImportedMeansNoScan(t *testing.T) {
	const src = `package p

func Get(x string) string { return x } // a local function that happens to be named Get

func f() { Get("x") }
`
	if findings := analyze(t, Config{}, src); len(findings) != 0 {
		t.Fatalf("a local Get() must not be mistaken for http.Get: %v", findings)
	}
}

func TestHighComplexity(t *testing.T) {
	const src = `package p

func simple(x int) int {
	if x > 0 {
		return 1
	}
	return 0
}
`
	if findings := analyze(t, Config{ComplexityThreshold: 5}, src); len(findings) != 0 {
		t.Fatalf("complexity 2 must not be flagged at threshold 5: %v", findings)
	}

	findings := analyze(t, Config{ComplexityThreshold: 1}, src)
	if len(findings) != 1 || findings[0].CheckID != "high-complexity" {
		t.Fatalf("findings = %v, want one high-complexity at threshold 1", findings)
	}
	f := findings[0]
	if f.Line != 3 || f.EndLine != 8 {
		t.Errorf("line/endLine = %d/%d, want the function's full span 3-8", f.Line, f.EndLine)
	}
	if !contains(f.Evidence, "complexity 2") && !contains(f.Evidence, "is 2") {
		t.Errorf("evidence = %q, want it to state the measured complexity", f.Evidence)
	}
}

func TestHighComplexityCountsBranchesAndLogicalOperators(t *testing.T) {
	const src = `package p

func f(a, b, c bool) int {
	switch {
	case a && b:
		return 1
	case b || c:
		return 2
	}
	for i := 0; i < 10; i++ {
		if i == 5 {
			continue
		}
	}
	return 0
}
`
	// base 1 + 2 case clauses + 2 (&&,||) + 1 for + 1 if = 7
	findings := analyze(t, Config{ComplexityThreshold: 6}, src)
	if len(findings) != 1 {
		t.Fatalf("findings = %v, want exactly one at threshold 6", findings)
	}
	if !contains(findings[0].Evidence, "is 7") {
		t.Fatalf("evidence = %q, want complexity 7", findings[0].Evidence)
	}
	if findings := analyze(t, Config{ComplexityThreshold: 7}, src); len(findings) != 0 {
		t.Fatalf("complexity 7 must not be flagged at threshold 7 (only strictly greater): %v", findings)
	}
}

func TestDefaultComplexityThreshold(t *testing.T) {
	src := "package p\n\nfunc f(n int) int {\n"
	for i := 0; i < defaultComplexityThreshold; i++ {
		src += "\tif n == " + itoa(i) + " { return " + itoa(i) + " }\n"
	}
	src += "\treturn -1\n}\n"

	if findings := analyze(t, Config{}, src); len(findings) != 1 || findings[0].CheckID != "high-complexity" {
		t.Fatalf("findings = %v, want exactly one high-complexity finding with the default threshold", findings)
	}
	if findings := analyze(t, Config{ComplexityThreshold: -1}, src); len(findings) != 1 {
		t.Fatalf("a non-positive configured threshold should fall back to the default")
	}
}

func TestFileThatFailsToParseIsSkippedNotFatal(t *testing.T) {
	findings, err := New(Config{}).Analyze(context.Background(), review.AnalysisInput{
		Files: []review.ChangedFile{
			{Path: "broken.go", Language: lang.Go, Content: "package p\nfunc ( {"},
			{Path: "fine.go", Language: lang.Go, Content: "package p\nimport \"os\"\nfunc f() { data, _ := os.ReadFile(\"a\"); _ = data }\n"},
		},
	})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if len(findings) != 1 || findings[0].File != "fine.go" {
		t.Fatalf("findings = %v, want only the valid file's finding", findings)
	}
}

func TestNonGoAndEmptyFilesAreSkipped(t *testing.T) {
	findings, err := New(Config{}).Analyze(context.Background(), review.AnalysisInput{
		Files: []review.ChangedFile{
			{Path: "app.ts", Language: "typescript", Content: "const x: any = 1;"},
			{Path: "empty.go", Language: lang.Go, Content: ""},
			{Path: "whitespace.go", Language: lang.Go, Content: "   \n\t\n"},
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
		Files: []review.ChangedFile{{Path: "a.go", Language: lang.Go, Content: "package p\n"}},
	})
	if err != context.Canceled {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

func TestFindingsLeaveSeverityAndSourceForTheServiceToFill(t *testing.T) {
	findings := analyze(t, Config{}, "package p\nimport \"os\"\nfunc f() { data, _ := os.ReadFile(\"a\"); _ = data }\n")
	if len(findings) != 1 {
		t.Fatalf("findings = %v", findings)
	}
	f := findings[0]
	if f.Severity != "" || f.Source != "" || f.Confidence != 0 || f.Lesson != "" {
		t.Fatalf("analyzer must not set severity/source/confidence/lesson (the service fills those from the skill): %+v", f)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (substr == "" || indexOf(s, substr) >= 0)
}

func indexOf(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := ""
	for n > 0 {
		digits = string(rune('0'+n%10)) + digits
		n /= 10
	}
	return digits
}
