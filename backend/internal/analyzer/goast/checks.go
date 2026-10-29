package goast

import (
	"fmt"
	"go/ast"
	"go/token"
	"strconv"
	"strings"

	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/review"
)

func importedName(f *ast.File, path string) (string, bool) {
	for _, imp := range f.Imports {
		p, err := strconv.Unquote(imp.Path.Value)
		if err != nil || p != path {
			continue
		}
		if imp.Name == nil {
			parts := strings.Split(path, "/")
			return parts[len(parts)-1], true
		}
		if imp.Name.Name == "_" || imp.Name.Name == "." {
			return "", false
		}
		return imp.Name.Name, true
	}
	return "", false
}

// isSelector reports whether e is the expression pkg.name.
func isSelector(e ast.Expr, pkg, name string) bool {
	sel, ok := e.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	return ok && id.Name == pkg && sel.Sel.Name == name
}

func funcLabel(fd *ast.FuncDecl) string {
	if fd.Recv == nil || len(fd.Recv.List) == 0 {
		return fd.Name.Name
	}
	switch recv := fd.Recv.List[0].Type.(type) {
	case *ast.StarExpr:
		if id, ok := recv.X.(*ast.Ident); ok {
			return fmt.Sprintf("(*%s).%s", id.Name, fd.Name.Name)
		}
	case *ast.Ident:
		return fmt.Sprintf("(%s).%s", recv.Name, fd.Name.Name)
	}
	return fd.Name.Name
}

// --- ignored-error -----------------------------------------------------------

func checkIgnoredError(src *source, stmt *ast.AssignStmt) (review.Finding, bool) {
	if len(stmt.Lhs) < 2 || len(stmt.Rhs) != 1 {
		return review.Finding{}, false
	}
	last, ok := stmt.Lhs[len(stmt.Lhs)-1].(*ast.Ident)
	if !ok || last.Name != "_" {
		return review.Finding{}, false
	}
	call, ok := stmt.Rhs[0].(*ast.CallExpr)
	if !ok {
		return review.Finding{}, false
	}

	found := fmt.Sprintf("The result of `%s` is discarded with `_`, in the position Go convention reserves for an error.",
		src.snippet(call.Pos(), call.End()))
	why := "If that value is an error, the failure becomes silent: execution continues as if nothing went wrong, " +
		"and the real cause is much harder to trace later. (This check is syntactic: a call whose last result " +
		"genuinely is not an error, such as a `(value, ok)` lookup, can also trigger it — check the callee's signature.)"
	improvement := "Capture it as `err`, check `if err != nil`, and return or wrap it: `fmt.Errorf(\"...: %w\", err)`."

	return newFinding("ignored-error", src, stmt.Pos(), stmt.End(),
		src.snippet(stmt.Pos(), stmt.End()), found, why, improvement), true
}

// --- context-not-first -------------------------------------------------------

type param struct {
	typ ast.Expr
}

func flattenParams(fl *ast.FieldList) []param {
	if fl == nil {
		return nil
	}
	var out []param
	for _, f := range fl.List {
		n := len(f.Names)
		if n == 0 {
			n = 1
		}
		for range n {
			out = append(out, param{typ: f.Type})
		}
	}
	return out
}

func checkContextNotFirst(src *source, fd *ast.FuncDecl, ctxPkg string) (review.Finding, bool) {
	if fd.Body == nil {
		return review.Finding{}, false
	}
	index := -1
	for i, p := range flattenParams(fd.Type.Params) {
		if isSelector(p.typ, ctxPkg, "Context") {
			index = i
			break
		}
	}
	if index <= 0 {
		return review.Finding{}, false
	}

	name := funcLabel(fd)
	found := fmt.Sprintf("`%s` accepts context.Context as parameter %d, not the first.", name, index+1)
	why := "Go code conventionally puts ctx first, so callers, tools and readers can rely on it without checking each signature."
	improvement := fmt.Sprintf("Move context.Context to be the first parameter of %s.", name)

	return newFinding("context-not-first", src, fd.Pos(), fd.Type.End(),
		src.snippet(fd.Pos(), fd.Type.End()), found, why, improvement), true
}

// --- http-call-without-context ----------------------------------------------

var httpFuncsWithoutContext = map[string]bool{"Get": true, "Post": true, "Head": true, "PostForm": true}

func checkHTTPWithoutContext(src *source, call *ast.CallExpr, httpPkg string) (review.Finding, bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return review.Finding{}, false
	}
	id, ok := sel.X.(*ast.Ident)
	if !ok || id.Name != httpPkg || !httpFuncsWithoutContext[sel.Sel.Name] {
		return review.Finding{}, false
	}

	text := src.snippet(call.Pos(), call.End())
	found := fmt.Sprintf("`%s` has no way to accept a context, so the request cannot be cancelled or given a deadline.", text)
	why := "If the caller gives up or the server stalls, this call keeps running and holds a connection open for as long as it takes to fail or succeed."
	improvement := "Build the request with http.NewRequestWithContext(ctx, ...) and send it with an *http.Client."

	return newFinding("http-call-without-context", src, call.Pos(), call.End(), text, found, why, improvement), true
}

// --- high-complexity ----------------------------------------------------------

func checkHighComplexity(src *source, fd *ast.FuncDecl, threshold int) (review.Finding, bool) {
	if fd.Body == nil {
		return review.Finding{}, false
	}

	complexity := 1
	ast.Inspect(fd.Body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt, *ast.CaseClause, *ast.CommClause:
			complexity++
		case *ast.BinaryExpr:
			if node.Op == token.LAND || node.Op == token.LOR {
				complexity++
			}
		}
		return true
	})
	if complexity <= threshold {
		return review.Finding{}, false
	}

	name := funcLabel(fd)
	evidence := fmt.Sprintf("Cyclomatic complexity of %s is %d (threshold %d).", name, complexity, threshold)
	found := fmt.Sprintf("%s has %d independent paths through it.", name, complexity)
	why := "Each extra branch is another case to test and hold in your head. Functions this size tend to hide bugs in their rarely exercised paths."
	improvement := "Extract validation, branching or response handling into smaller, named functions."

	return newFinding("high-complexity", src, fd.Pos(), fd.End(), evidence, found, why, improvement), true
}
