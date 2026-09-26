package review

import (
	"strings"
	"testing"
)

func validFinding() Finding {
	return Finding{
		SkillID:     "go",
		CheckID:     "ignored-error",
		Source:      SourceDeterministic,
		Severity:    SeverityHigh,
		Confidence:  1,
		File:        "main.go",
		Line:        10,
		Evidence:    "data, _ := os.ReadFile(path)",
		Found:       "error ignored",
		Why:         "failure is silent",
		Improvement: "handle the error",
	}
}

func TestFindingValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Finding)
		wantErr string
	}{
		{"valid", func(*Finding) {}, ""},
		{"missing check id", func(f *Finding) { f.CheckID = "" }, "skill and check id"},
		{"bad severity", func(f *Finding) { f.Severity = "urgent" }, "invalid severity"},
		{"bad source", func(f *Finding) { f.Source = "guess" }, "invalid source"},
		{"confidence too high", func(f *Finding) { f.Confidence = 1.5 }, "out of range"},
		{"deterministic below 1", func(f *Finding) { f.Confidence = 0.9 }, "must have confidence 1"},
		{"ai below 1 is fine", func(f *Finding) { f.Source = SourceAI; f.Confidence = 0.7 }, ""},
		{"no file", func(f *Finding) { f.File = "" }, "needs a file"},
		{"line zero", func(f *Finding) { f.Line = 0 }, "needs a file"},
		{"no evidence", func(f *Finding) { f.Evidence = "" }, "evidence is required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := validFinding()
			tt.mutate(&f)
			err := f.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}
