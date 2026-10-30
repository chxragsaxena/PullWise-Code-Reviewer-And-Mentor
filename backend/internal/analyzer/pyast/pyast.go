// Package pyast implements review.StaticAnalyzer for Python using tree-sitter.
package pyast

import (
	"context"
	"log/slog"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/python"

	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/lang"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/review"
)

// defaultComplexityThreshold matches the python skill's high-complexity check.
const defaultComplexityThreshold = 15

// Config tunes the analyzer.
type Config struct {
	ComplexityThreshold int
}

// Analyzer implements review.StaticAnalyzer for Python.
type Analyzer struct {
	complexityThreshold int
}

// New returns a Python analyzer.
func New(cfg Config) *Analyzer {
	threshold := cfg.ComplexityThreshold
	if threshold <= 0 {
		threshold = defaultComplexityThreshold
	}
	return &Analyzer{complexityThreshold: threshold}
}

// Analyze runs every check against each changed Python file.
func (a *Analyzer) Analyze(ctx context.Context, input review.AnalysisInput) ([]review.Finding, error) {
	var findings []review.Finding
	for _, file := range input.Files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if file.Language != lang.Python || strings.TrimSpace(file.Content) == "" {
			continue
		}
		found, err := a.analyzeFile(ctx, file)
		if err != nil {
			slog.Warn("pyast: skipping a file that failed to parse", "path", file.Path, "error", err)
			continue
		}
		findings = append(findings, found...)
	}
	return findings, nil
}

func (a *Analyzer) analyzeFile(ctx context.Context, file review.ChangedFile) ([]review.Finding, error) {
	content := []byte(file.Content)
	parser := sitter.NewParser()
	defer parser.Close()
	parser.SetLanguage(python.GetLanguage())

	tree, err := parser.ParseCtx(ctx, nil, content)
	if err != nil {
		return nil, err
	}
	defer tree.Close()

	src := &source{content: content, path: file.Path}
	var findings []review.Finding
	add := func(f review.Finding, ok bool) {
		if ok {
			findings = append(findings, f)
		}
	}

	walk(tree.RootNode(), func(n *sitter.Node) {
		switch n.Type() {
		case "except_clause":
			add(checkBareExcept(src, n))
			add(checkSwallowedException(src, n))
		case "default_parameter", "typed_default_parameter":
			add(checkMutableDefault(src, n))
		case "call":
			add(checkRequestWithoutTimeout(src, n))
		case "function_definition":
			add(checkHighComplexity(src, n, a.complexityThreshold))
		}
	})
	return findings, nil
}

func walk(n *sitter.Node, visit func(*sitter.Node)) {
	visit(n)
	for i := 0; i < int(n.NamedChildCount()); i++ {
		walk(n.NamedChild(i), visit)
	}
}

// source turns tree-sitter positions into the fields a Finding needs.
type source struct {
	content []byte
	path    string
}

func (s *source) line(n *sitter.Node) int { return int(n.StartPoint().Row) + 1 }

func (s *source) endLine(n *sitter.Node) int {
	start, end := n.StartPoint().Row, n.EndPoint().Row
	if start == end {
		return 0
	}
	return int(end) + 1
}

func (s *source) text(n *sitter.Node) string { return n.Content(s.content) }

func newFinding(checkID string, src *source, n *sitter.Node, evidence, found, why, improvement string) review.Finding {
	return review.Finding{
		SkillID: "python", CheckID: checkID, File: src.path,
		Line: src.line(n), EndLine: src.endLine(n),
		Evidence: evidence, Found: found, Why: why, Improvement: improvement,
	}
}
