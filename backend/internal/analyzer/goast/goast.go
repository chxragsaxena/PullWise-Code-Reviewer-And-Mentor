// Package goast implements review.StaticAnalyzer for Go source using go/parser and go/ast.
package goast

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"log/slog"
	"strings"

	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/lang"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/review"
)

// defaultComplexityThreshold matches the go skill's high-complexity check.
const defaultComplexityThreshold = 15

// Config tunes the analyzer.
type Config struct {
	ComplexityThreshold int
}

// Analyzer implements review.StaticAnalyzer for Go.
type Analyzer struct {
	complexityThreshold int
}

// New returns a Go analyzer.
func New(cfg Config) *Analyzer {
	threshold := cfg.ComplexityThreshold
	if threshold <= 0 {
		threshold = defaultComplexityThreshold
	}
	return &Analyzer{complexityThreshold: threshold}
}

// Analyze runs every check against each changed Go file.
func (a *Analyzer) Analyze(ctx context.Context, input review.AnalysisInput) ([]review.Finding, error) {
	var findings []review.Finding
	for _, file := range input.Files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if file.Language != lang.Go || strings.TrimSpace(file.Content) == "" {
			continue
		}
		findings = append(findings, a.analyzeFile(file)...)
	}
	return findings, nil
}

func (a *Analyzer) analyzeFile(file review.ChangedFile) []review.Finding {
	fset := token.NewFileSet()
	astFile, err := parser.ParseFile(fset, file.Path, file.Content, parser.ParseComments)
	if err != nil {
		slog.Warn("goast: skipping a file that failed to parse", "path", file.Path, "error", err)
		return nil
	}

	ctxPkg, hasCtxPkg := importedName(astFile, "context")
	httpPkg, hasHTTPPkg := importedName(astFile, "net/http")
	src := &source{fset: fset, content: file.Content, path: file.Path}

	var findings []review.Finding
	ast.Inspect(astFile, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.AssignStmt:
			if f, ok := checkIgnoredError(src, node); ok {
				findings = append(findings, f)
			}
		case *ast.CallExpr:
			if hasHTTPPkg {
				if f, ok := checkHTTPWithoutContext(src, node, httpPkg); ok {
					findings = append(findings, f)
				}
			}
		case *ast.FuncDecl:
			if hasCtxPkg {
				if f, ok := checkContextNotFirst(src, node, ctxPkg); ok {
					findings = append(findings, f)
				}
			}
			if f, ok := checkHighComplexity(src, node, a.complexityThreshold); ok {
				findings = append(findings, f)
			}
		}
		return true
	})
	return findings
}

// source turns AST positions into the fields a Finding needs.
type source struct {
	fset    *token.FileSet
	content string
	path    string
}

func (s *source) line(pos token.Pos) int { return s.fset.Position(pos).Line }

func (s *source) endLine(start, end token.Pos) int {
	if a, b := s.line(start), s.line(end); a != b {
		return b
	}
	return 0
}

func (s *source) snippet(start, end token.Pos) string {
	a, b := s.fset.Position(start).Offset, s.fset.Position(end).Offset
	if a < 0 || b > len(s.content) || a >= b {
		return ""
	}
	const max = 200
	text := strings.TrimSpace(s.content[a:b])
	if len(text) > max {
		text = text[:max] + " ..."
	}
	return text
}

func newFinding(checkID string, src *source, start, end token.Pos, evidence, found, why, improvement string) review.Finding {
	return review.Finding{
		SkillID: "go", CheckID: checkID, File: src.path,
		Line: src.line(start), EndLine: src.endLine(start, end),
		Evidence: evidence, Found: found, Why: why, Improvement: improvement,
	}
}
