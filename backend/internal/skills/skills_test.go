package skills

import (
	"os"
	"strings"
	"testing"
	"testing/fstest"
)

const validYAML = `
id: demo
name: Demo
version: 1
languages: [go]
checks:
  - id: ignored-error
    title: Error ignored
    kind: deterministic
    analyzer: semgrep
    severity: high
    lesson: errors
  - id: too-complex
    title: Too complex
    kind: deterministic
    analyzer: metric
    threshold: 15
    severity: medium
    lesson: errors
  - id: leak
    title: Leak risk
    kind: ai
    severity: high
    lesson: errors
`

func demoFS(yaml string) fstest.MapFS {
	return fstest.MapFS{
		"demo/skill.yaml":        {Data: []byte(yaml)},
		"demo/lessons/errors.md": {Data: []byte("# Errors")},
	}
}

func TestLoadValid(t *testing.T) {
	got, err := Load(demoFS(validYAML))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got) != 1 || got[0].ID != "demo" || len(got[0].Checks) != 3 {
		t.Fatalf("unexpected result: %+v", got)
	}
	if !got[0].AppliesTo("go") || got[0].AppliesTo("python") {
		t.Fatal("AppliesTo gave the wrong answer")
	}
	lesson, err := got[0].Lesson("errors")
	if err != nil || lesson != "# Errors" {
		t.Fatalf("Lesson = %q, %v", lesson, err)
	}
}

func TestAppliesTo(t *testing.T) {
	tests := []struct {
		name      string
		languages []string
		language  string
		want      bool
	}{
		{"listed language", []string{"go"}, "go", true},
		{"unlisted language", []string{"go"}, "python", false},
		{"one of several", []string{"go", "typescript"}, "typescript", true},
		{"wildcard matches a language the skill never named", []string{AnyLanguage}, "python", true},
		{"wildcard matches the others too", []string{AnyLanguage}, "go", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := (Skill{Languages: tt.languages}).AppliesTo(tt.language); got != tt.want {
				t.Fatalf("AppliesTo(%q) with %v = %v, want %v", tt.language, tt.languages, got, tt.want)
			}
		})
	}
}

func TestLoadSkipsDirectoriesWithoutSkillFile(t *testing.T) {
	fsys := demoFS(validYAML)
	fsys["notes/readme.md"] = &fstest.MapFile{Data: []byte("hi")}
	got, err := Load(fsys)
	if err != nil || len(got) != 1 {
		t.Fatalf("got %v, %v; want exactly the demo skill", got, err)
	}
}

func TestLoadRejectsInvalidSkills(t *testing.T) {
	tests := []struct {
		name    string
		replace [2]string
		want    string
	}{
		{"unknown field", [2]string{"version: 1", "version: 1\nbogus: true"}, "bogus"},
		{"bad version", [2]string{"version: 1", "version: 0"}, "version must be"},
		{"no languages", [2]string{"languages: [go]", "languages: []"}, "at least one language"},
		{"bad severity", [2]string{"severity: high\n    lesson: errors\n  - id: too", "severity: urgent\n    lesson: errors\n  - id: too"}, "invalid severity"},
		{"bad kind", [2]string{"kind: ai", "kind: magic"}, "invalid kind"},
		{"unknown analyzer", [2]string{"analyzer: semgrep", "analyzer: grep"}, "known analyzer"},
		{"metric without threshold", [2]string{"threshold: 15", "threshold: 0"}, "need a threshold"},
		{"missing lesson file", [2]string{"lesson: errors\n  - id: too", "lesson: nope\n  - id: too"}, "lesson \"nope\""},
		{"duplicate check id", [2]string{"id: too-complex", "id: ignored-error"}, "duplicated"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			yaml := strings.Replace(validYAML, tt.replace[0], tt.replace[1], 1)
			if yaml == validYAML {
				t.Fatal("test bug: replacement did not change the YAML")
			}
			_, err := Load(demoFS(yaml))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want it to contain %q", err, tt.want)
			}
		})
	}
}

func TestLoadRejectsDuplicateSkillIDs(t *testing.T) {
	fsys := demoFS(validYAML)
	fsys["other/skill.yaml"] = &fstest.MapFile{Data: []byte(validYAML)}
	fsys["other/lessons/errors.md"] = &fstest.MapFile{Data: []byte("x")}
	if _, err := Load(fsys); err == nil || !strings.Contains(err.Error(), "used by both") {
		t.Fatalf("error = %v, want duplicate skill id error", err)
	}
}

// TestRepositorySkills keeps the real skills/ directory valid.
func TestRepositorySkills(t *testing.T) {
	got, err := Load(os.DirFS("../../skills"))
	if err != nil {
		t.Fatalf("repository skills are invalid: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("expected at least one skill in skills/")
	}
}
