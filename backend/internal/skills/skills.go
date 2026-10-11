// Package skills loads review skills from disk. A skill is a skill.yaml that
// says what to check, plus lessons/*.md that say what to teach.
package skills

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"

	"gopkg.in/yaml.v3"

	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/review"
)

// errNoSkillFile marks a directory that holds no skill.yaml.
var errNoSkillFile = errors.New("no skill.yaml")

// Kind says how a check is evaluated.
type Kind string

const (
	KindDeterministic Kind = "deterministic"
	KindAI            Kind = "ai"
)

// Analyzer names the engine behind a deterministic check.
type Analyzer string

const (
	AnalyzerSemgrep    Analyzer = "semgrep"
	AnalyzerTreeSitter Analyzer = "treesitter"
	AnalyzerMetric     Analyzer = "metric"
	// AnalyzerGoAST is internal/analyzer/goast: Go's own go/parser + go/ast,
	// used instead of Tree-sitter for Go specifically (see docs/TRACKER.md).
	AnalyzerGoAST Analyzer = "goast"
)

// Check is one thing a skill looks for.
type Check struct {
	ID        string          `yaml:"id"`
	Title     string          `yaml:"title"`
	Kind      Kind            `yaml:"kind"`
	Analyzer  Analyzer        `yaml:"analyzer"`
	Threshold int             `yaml:"threshold"`
	Severity  review.Severity `yaml:"severity"`
	Lesson    string          `yaml:"lesson"`
}

// Skill is a packaged review capability.
type Skill struct {
	ID          string   `yaml:"id"`
	Name        string   `yaml:"name"`
	Version     int      `yaml:"version"`
	Languages   []string `yaml:"languages"`
	Description string   `yaml:"description"`
	Checks      []Check  `yaml:"checks"`

	fsys fs.FS
	dir  string
}

// Lesson returns the Markdown text of a lesson file belonging to this skill.
func (s Skill) Lesson(name string) (string, error) {
	data, err := fs.ReadFile(s.fsys, path.Join(s.dir, "lessons", name+".md"))
	if err != nil {
		return "", fmt.Errorf("skill %q: read lesson %q: %w", s.ID, name, err)
	}
	return string(data), nil
}

// Check returns the check with the given id.
func (s Skill) Check(id string) (Check, bool) {
	for _, c := range s.Checks {
		if c.ID == id {
			return c, true
		}
	}
	return Check{}, false
}

// AppliesTo reports whether the skill covers the given language.
func (s Skill) AppliesTo(language string) bool {
	for _, l := range s.Languages {
		if l == language {
			return true
		}
	}
	return false
}

// Load reads every <dir>/*/skill.yaml from fsys, validates each skill and
// returns them sorted by ID. Any invalid skill fails the whole load, so
// mistakes surface at startup rather than mid-review.
func Load(fsys fs.FS) ([]Skill, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("read skills directory: %w", err)
	}

	var loaded []Skill
	seen := make(map[string]string)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		s, err := loadOne(fsys, e.Name())
		if errors.Is(err, errNoSkillFile) {
			continue // a directory without skill.yaml is not a skill
		}
		if err != nil {
			return nil, err
		}
		if prev, dup := seen[s.ID]; dup {
			return nil, fmt.Errorf("skill id %q used by both %q and %q", s.ID, prev, e.Name())
		}
		seen[s.ID] = e.Name()
		loaded = append(loaded, s)
	}

	sort.Slice(loaded, func(i, j int) bool { return loaded[i].ID < loaded[j].ID })
	return loaded, nil
}

func loadOne(fsys fs.FS, dir string) (Skill, error) {
	file := path.Join(dir, "skill.yaml")
	data, err := fs.ReadFile(fsys, file)
	if errors.Is(err, fs.ErrNotExist) {
		return Skill{}, errNoSkillFile
	}
	if err != nil {
		return Skill{}, fmt.Errorf("read %s: %w", file, err)
	}

	var s Skill
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&s); err != nil {
		return Skill{}, fmt.Errorf("parse %s: %w", file, err)
	}
	s.fsys, s.dir = fsys, dir

	if err := s.validate(); err != nil {
		return Skill{}, fmt.Errorf("%s: %w", file, err)
	}
	return s, nil
}

func (s Skill) validate() error {
	if s.ID == "" || s.Name == "" {
		return errors.New("skill needs an id and a name")
	}
	if s.Version < 1 {
		return errors.New("version must be at least 1")
	}
	if len(s.Languages) == 0 {
		return errors.New("skill must list at least one language")
	}
	if len(s.Checks) == 0 {
		return errors.New("skill must define at least one check")
	}

	ids := make(map[string]bool, len(s.Checks))
	for _, c := range s.Checks {
		if err := s.validateCheck(c); err != nil {
			return fmt.Errorf("check %q: %w", c.ID, err)
		}
		if ids[c.ID] {
			return fmt.Errorf("check id %q is duplicated", c.ID)
		}
		ids[c.ID] = true
	}
	return nil
}

func (s Skill) validateCheck(c Check) error {
	if c.ID == "" || c.Title == "" {
		return errors.New("needs an id and a title")
	}
	if !c.Severity.Valid() {
		return fmt.Errorf("invalid severity %q", c.Severity)
	}

	switch c.Kind {
	case KindDeterministic:
		switch c.Analyzer {
		case AnalyzerSemgrep, AnalyzerTreeSitter, AnalyzerGoAST:
		case AnalyzerMetric:
			if c.Threshold < 1 {
				return errors.New("metric checks need a threshold")
			}
		default:
			return fmt.Errorf("deterministic checks need a known analyzer, got %q", c.Analyzer)
		}
	case KindAI:
		if c.Analyzer != "" {
			return errors.New("ai checks must not set an analyzer")
		}
	default:
		return fmt.Errorf("invalid kind %q", c.Kind)
	}

	if c.Lesson == "" {
		return errors.New("needs a lesson")
	}
	if _, err := fs.Stat(s.fsys, path.Join(s.dir, "lessons", c.Lesson+".md")); err != nil {
		return fmt.Errorf("lesson %q: %w", c.Lesson, err)
	}
	return nil
}
