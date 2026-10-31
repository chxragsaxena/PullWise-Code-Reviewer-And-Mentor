package service

import (
	"reflect"
	"sort"
	"testing"

	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/review"
)

func lineSet(m map[int]bool) []int {
	var out []int
	for l := range m {
		out = append(out, l)
	}
	sort.Ints(out)
	return out
}

func TestTouchedLinesReadsAddedAndRemovedLines(t *testing.T) {
	tests := []struct {
		name  string
		patch string
		want  []int
	}{
		{"empty patch means nothing is known", "", nil},
		{"an unreadable patch means nothing is known", "@@", nil},
		{"a hunk with only context means nothing is known", "@@ -1,2 +1,2 @@\n a\n b", nil},
		{"added lines", "@@ -1,2 +1,4 @@\n a\n+b\n+c\n d", []int{2, 3}},
		{"a removal marks the line that now follows it", "@@ -1,3 +1,2 @@\n a\n-b\n c", []int{2}},
		{"replaced line", "@@ -5,3 +5,3 @@ func f() {\n x\n-old\n+new\n y", []int{6}},
		{"two hunks", "@@ -1,2 +1,3 @@\n a\n+b\n c\n@@ -10,2 +11,3 @@\n m\n+n\n o", []int{2, 12}},
		{"a new file", "@@ -0,0 +1,3 @@\n+a\n+b\n+c", []int{1, 2, 3}},
		{"no newline marker is ignored", "@@ -1 +1 @@\n-a\n\\ No newline at end of file\n+b\n\\ No newline at end of file", []int{1}},
		{"header without a count", "@@ -3 +3 @@\n-x\n+y", []int{3}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := lineSet(touchedLines(tt.patch))
			if !reflect.DeepEqual(got, tt.want) && !(len(got) == 0 && len(tt.want) == 0) {
				t.Fatalf("touchedLines = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestScopeToChangesKeepsOnlyFindingsOnChangedCode(t *testing.T) {
	files := []review.ChangedFile{
		{Path: "a.go", Patch: "@@ -10,3 +10,4 @@\n x\n+added\n y\n z"}, // touches line 11
		{Path: "big.go", Patch: ""},                                    // no diff available
		{Path: "b.go", Patch: "@@ -1,2 +1,2 @@\n-old\n+new\n same"},    // touches line 1
	}
	findings := []review.Finding{
		{File: "a.go", Line: 11, CheckID: "on-added-line"},
		{File: "a.go", Line: 50, CheckID: "far-away"},
		{File: "a.go", Line: 5, EndLine: 20, CheckID: "function-spanning-the-change"},
		{File: "a.go", Line: 20, EndLine: 30, CheckID: "function-after-the-change"},
		{File: "big.go", Line: 400, CheckID: "no-diff-so-kept"},
		{File: "other.go", Line: 3, CheckID: "unknown-file-so-kept"},
		{File: "b.go", Line: 1, CheckID: "replaced-line"},
		{File: "b.go", Line: 2, CheckID: "unchanged-line-next-to-it"},
	}

	kept, dropped := scopeToChanges(findings, files)

	var got []string
	for _, f := range kept {
		got = append(got, f.CheckID)
	}
	want := []string{"on-added-line", "function-spanning-the-change", "no-diff-so-kept", "unknown-file-so-kept", "replaced-line"}
	if !reflect.DeepEqual(got, want) || dropped != 3 {
		t.Fatalf("kept %v (dropped %d), want %v (dropped 3)", got, dropped, want)
	}
}

func TestScopeToChangesLeavesTheInputAlone(t *testing.T) {
	in := []review.Finding{{File: "a.go", Line: 9, CheckID: "x"}}
	files := []review.ChangedFile{{Path: "a.go", Patch: "@@ -1,1 +1,2 @@\n a\n+b"}}
	kept, dropped := scopeToChanges(in, files)
	if len(kept) != 0 || dropped != 1 || len(in) != 1 || in[0].CheckID != "x" {
		t.Fatalf("kept=%v dropped=%d in=%v", kept, dropped, in)
	}
}
