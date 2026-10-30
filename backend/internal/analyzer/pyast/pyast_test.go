package pyast

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/lang"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/review"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/skills"
)

func analyze(t *testing.T, cfg Config, content string) []review.Finding {
	t.Helper()
	findings, err := New(cfg).Analyze(context.Background(), review.AnalysisInput{
		Files: []review.ChangedFile{{Path: "app.py", Language: lang.Python, Content: content}},
	})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	return findings
}

// linesOf returns the lines of the findings for one check.
func linesOf(findings []review.Finding, check string) []int {
	var lines []int
	for _, f := range findings {
		if f.CheckID == check {
			lines = append(lines, f.Line)
		}
	}
	return lines
}

func sameInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestBareExceptFlagsOnlyClausesWithNoType(t *testing.T) {
	const src = `
try:
    work()
except:
    log()
try:
    work()
except ValueError:
    log()
except (KeyError, TypeError) as e:
    log(e)
except Exception:
    log()
`
	got := linesOf(analyze(t, Config{}, src), "bare-except")
	if !sameInts(got, []int{4}) {
		t.Fatalf("bare-except lines = %v, want [4]", got)
	}
}

func TestSwallowedExceptionFlagsPassAndEllipsisOnly(t *testing.T) {
	const src = `
try:
    work()
except ValueError:
    pass
try:
    work()
except KeyError:
    ...
try:
    work()
except TypeError:
    log("x")
try:
    work()
except OSError:
    pass
    log("later")
`
	got := linesOf(analyze(t, Config{}, src), "swallowed-exception")
	if !sameInts(got, []int{4, 8}) {
		t.Fatalf("swallowed-exception lines = %v, want [4 8]", got)
	}
}

func TestBareExceptWithPassIsBothProblems(t *testing.T) {
	findings := analyze(t, Config{}, "try:\n    work()\nexcept:\n    pass\n")
	if len(linesOf(findings, "bare-except")) != 1 || len(linesOf(findings, "swallowed-exception")) != 1 {
		t.Fatalf("findings = %+v, want one of each", findings)
	}
}

func TestMutableDefaultArgument(t *testing.T) {
	const src = `
def good(a, b=None, c=0, d="x", e=(), f=frozenset(), g=len):
    pass
def bad(a, b=[], c={}, d=set(), e: list = list(), f=dict(), g={1, 2}, h=[x for x in y]):
    pass
`
	findings := analyze(t, Config{}, src)
	var params []string
	for _, f := range findings {
		if f.CheckID == "mutable-default-argument" {
			params = append(params, f.Evidence)
		}
	}
	want := "b=[] c={} d=set() e: list = list() f=dict() g={1, 2} h=[x for x in y]"
	if got := strings.Join(params, " "); got != want {
		t.Fatalf("flagged %q\nwant    %q", got, want)
	}
	if !strings.Contains(findings[0].Improvement, "None") {
		t.Errorf("improvement = %q, want it to suggest None", findings[0].Improvement)
	}
}

func TestRequestWithoutTimeout(t *testing.T) {
	const src = `
import requests
requests.get(url)
requests.post(url, json=data)
requests.get(url, timeout=5)
requests.put(url, data, timeout=(3, 10))
requests.get(url, **options)
session.get(url)
client.get(url)
requests.Session()
`
	got := linesOf(analyze(t, Config{}, src), "request-without-timeout")
	if !sameInts(got, []int{3, 4}) {
		t.Fatalf("request-without-timeout lines = %v, want [3 4]", got)
	}
}

func TestHighComplexityCountsBranchesAndBooleans(t *testing.T) {
	const src = `
def simple(a):
    return a

def busy(a, b, c):
    if a and b or c:
        pass
    elif b:
        pass
    for i in a:
        while b:
            pass
    try:
        pass
    except ValueError:
        pass
    x = 1 if a else 2
    match c:
        case 1:
            pass
`
	// 1 + if + and + or + elif + for + while + except + conditional + case = 10
	if got := linesOf(analyze(t, Config{ComplexityThreshold: 10}, src), "high-complexity"); len(got) != 0 {
		t.Fatalf("complexity 10 flagged at threshold 10: %v", got)
	}
	findings := analyze(t, Config{ComplexityThreshold: 9}, src)
	got := linesOf(findings, "high-complexity")
	if !sameInts(got, []int{5}) {
		t.Fatalf("high-complexity lines = %v, want [5]", got)
	}
	if !strings.Contains(findings[0].Evidence, "is 10") || !strings.Contains(findings[0].Evidence, "`busy`") {
		t.Errorf("evidence = %q", findings[0].Evidence)
	}
}

func TestDefaultThresholdIs15(t *testing.T) {
	var b strings.Builder
	b.WriteString("def f(a):\n")
	for i := 0; i < 15; i++ {
		fmt.Fprintf(&b, "    if a == %d:\n        pass\n", i)
	}
	// 1 + 15 ifs = 16 > 15
	if got := linesOf(analyze(t, Config{}, b.String()), "high-complexity"); len(got) != 1 {
		t.Fatalf("complexity 16 with the default threshold: flagged %v", got)
	}
}

func TestFindingFieldsAreFilledIn(t *testing.T) {
	findings := analyze(t, Config{}, "try:\n    work()\nexcept:\n    log()\n")
	if len(findings) != 1 {
		t.Fatalf("findings = %+v", findings)
	}
	f := findings[0]
	if f.SkillID != "python" || f.File != "app.py" || f.Line != 3 || f.EndLine != 4 ||
		f.Evidence == "" || f.Found == "" || f.Why == "" || f.Improvement == "" {
		t.Fatalf("finding = %+v", f)
	}
	if strings.Contains(f.Evidence, "\n") {
		t.Errorf("evidence should be one line, got %q", f.Evidence)
	}
}

// Every check the analyzer can emit must exist in the python skill as a pyast check with a lesson.
func TestEveryFindingMatchesACheckInThePythonSkill(t *testing.T) {
	loaded, err := skills.Load(os.DirFS("../../../skills"))
	if err != nil {
		t.Fatal(err)
	}
	var python skills.Skill
	for _, sk := range loaded {
		if sk.ID == "python" {
			python = sk
		}
	}
	if python.ID == "" || !python.AppliesTo(lang.Python) {
		t.Fatal("skills/python is missing or does not apply to Python")
	}

	const src = `
def f(a, b=[]):
    try:
        requests.get(a)
    except:
        pass
` + "    if a:\n        pass\n"
	seen := map[string]bool{}
	for _, f := range analyze(t, Config{ComplexityThreshold: 1}, src) {
		seen[f.CheckID] = true
		check, ok := python.Check(f.CheckID)
		if !ok || check.Kind != skills.KindDeterministic || check.Analyzer != skills.AnalyzerPyAST {
			t.Errorf("finding %q has no matching pyast check in skills/python", f.CheckID)
		}
		if _, err := python.Lesson(check.Lesson); err != nil {
			t.Errorf("check %q: lesson %q: %v", f.CheckID, check.Lesson, err)
		}
	}
	for _, id := range []string{"bare-except", "swallowed-exception", "mutable-default-argument", "request-without-timeout", "high-complexity"} {
		if !seen[id] {
			t.Errorf("the sample did not trigger %q", id)
		}
	}
}

func TestOtherLanguagesAndEmptyFilesAreSkipped(t *testing.T) {
	findings, err := New(Config{}).Analyze(context.Background(), review.AnalysisInput{Files: []review.ChangedFile{
		{Path: "a.go", Language: lang.Go, Content: "except:"},
		{Path: "empty.py", Language: lang.Python, Content: "  \n"},
	}})
	if err != nil || len(findings) != 0 {
		t.Fatalf("findings = %+v, err = %v", findings, err)
	}
}

func TestCancelledContextStopsAnalysis(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := New(Config{}).Analyze(ctx, review.AnalysisInput{Files: []review.ChangedFile{
		{Path: "a.py", Language: lang.Python, Content: "x = 1"},
	}})
	if err == nil {
		t.Fatal("a cancelled context must stop the analysis")
	}
}

func TestBrokenPythonStillYieldsWhatItCanParse(t *testing.T) {
	// tree-sitter recovers from syntax errors; the valid parts are still checked.
	findings := analyze(t, Config{}, "def (:\n\ntry:\n    x()\nexcept:\n    y()\n")
	if len(linesOf(findings, "bare-except")) != 1 {
		t.Fatalf("findings = %+v, want the bare except found despite the syntax error", findings)
	}
}
