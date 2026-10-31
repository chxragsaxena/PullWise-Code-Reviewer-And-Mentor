package service

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/review"
)

var hunkHeader = regexp.MustCompile(`^@@ -\d+(?:,\d+)? \+(\d+)(?:,\d+)? @@`)

// touchedLines returns the lines of the new file that a unified diff adds or removes. A removal marks the
// line that now follows it. A patch with no readable changes returns nil: nothing is known, so nothing
// can be ruled out.
func touchedLines(patch string) map[int]bool {
	if strings.TrimSpace(patch) == "" {
		return nil
	}
	touched := map[int]bool{}
	line := 0
	for _, l := range strings.Split(patch, "\n") {
		if m := hunkHeader.FindStringSubmatch(l); m != nil {
			line, _ = strconv.Atoi(m[1])
			continue
		}
		if line == 0 || l == "" {
			continue
		}
		switch l[0] {
		case '+':
			touched[line] = true
			line++
		case '-':
			touched[line] = true
		case ' ':
			line++
		}
	}
	if len(touched) == 0 {
		return nil
	}
	return touched
}

// scopeToChanges drops findings that sit entirely in code the pull request did not change, and returns
// how many it dropped. A finding spanning several lines stays if any of them changed. Findings about files
// without a usable diff are kept.
func scopeToChanges(findings []review.Finding, files []review.ChangedFile) ([]review.Finding, int) {
	touched := make(map[string]map[int]bool, len(files))
	for _, f := range files {
		if t := touchedLines(f.Patch); t != nil {
			touched[f.Path] = t
		}
	}

	kept := findings[:0:0]
	dropped := 0
	for _, f := range findings {
		lines, known := touched[f.File]
		if !known || overlaps(lines, f.Line, f.EndLine) {
			kept = append(kept, f)
			continue
		}
		dropped++
	}
	return kept, dropped
}

func overlaps(lines map[int]bool, start, end int) bool {
	if end < start {
		end = start
	}
	for l := start; l <= end; l++ {
		if lines[l] {
			return true
		}
	}
	return false
}
