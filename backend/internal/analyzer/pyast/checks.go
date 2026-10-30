package pyast

import (
	"fmt"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/review"
)

// excerpt shortens evidence to its first line, so a long block does not fill the finding.
func excerpt(src *source, n *sitter.Node) string {
	text := src.text(n)
	for i, r := range text {
		if r == '\n' {
			return text[:i] + " ..."
		}
	}
	return text
}

// exceptBody returns the block of an except clause: always its last named child.
func exceptBody(n *sitter.Node) *sitter.Node {
	if n.NamedChildCount() == 0 {
		return nil
	}
	last := n.NamedChild(int(n.NamedChildCount()) - 1)
	if last.Type() != "block" {
		return nil
	}
	return last
}

// --- bare-except ---------------------------------------------------------------

func checkBareExcept(src *source, clause *sitter.Node) (review.Finding, bool) {
	// A bare clause holds only its block; a typed one also holds the exception expression.
	if clause.NamedChildCount() != 1 || exceptBody(clause) == nil {
		return review.Finding{}, false
	}
	found := "`except:` with no exception type catches everything, including KeyboardInterrupt and SystemExit."
	why := "Catching every exception hides real bugs and makes the program impossible to interrupt with Ctrl-C or to shut down cleanly; the failure surfaces somewhere else, much later."
	improvement := "Catch the specific exceptions you can handle, for example `except ValueError:`. If you truly need a catch-all, use `except Exception:` and log it."
	return newFinding("bare-except", src, clause, excerpt(src, clause), found, why, improvement), true
}

// --- swallowed-exception -------------------------------------------------------

func checkSwallowedException(src *source, clause *sitter.Node) (review.Finding, bool) {
	body := exceptBody(clause)
	if body == nil || body.NamedChildCount() != 1 {
		return review.Finding{}, false
	}
	only := body.NamedChild(0)
	switch {
	case only.Type() == "pass_statement":
	case only.Type() == "expression_statement" && only.NamedChildCount() == 1 && only.NamedChild(0).Type() == "ellipsis":
	default:
		return review.Finding{}, false
	}
	found := "This `except` block does nothing, so the error is silently ignored."
	why := "When the operation fails, nothing records it and the code carries on as if it had worked, which turns a clear error into confusing wrong behaviour later."
	improvement := "Handle the error, or at least log it with `logging.exception(...)`. If ignoring it is intended, say why in a comment and catch only the specific exception."
	return newFinding("swallowed-exception", src, clause, excerpt(src, clause), found, why, improvement), true
}

// --- mutable-default-argument --------------------------------------------------

var mutableConstructors = map[string]bool{"list": true, "dict": true, "set": true}

func checkMutableDefault(src *source, param *sitter.Node) (review.Finding, bool) {
	value := param.ChildByFieldName("value")
	if value == nil {
		return review.Finding{}, false
	}
	switch value.Type() {
	case "list", "dictionary", "set", "list_comprehension", "dictionary_comprehension", "set_comprehension":
	case "call":
		fn := value.ChildByFieldName("function")
		if fn == nil || fn.Type() != "identifier" || !mutableConstructors[src.text(fn)] {
			return review.Finding{}, false
		}
	default:
		return review.Finding{}, false
	}

	text := src.text(param)
	name := "This parameter"
	if id := param.ChildByFieldName("name"); id != nil {
		name = fmt.Sprintf("Parameter `%s`", src.text(id))
	}
	found := fmt.Sprintf("%s defaults to a mutable value (`%s`).", name, src.text(value))
	why := "Python builds a default value once, when the function is defined, so every call that relies on it shares the same object. Changes made in one call show up in the next."
	improvement := "Default to `None` and create the value inside the function: `def f(items=None): items = [] if items is None else items`."
	return newFinding("mutable-default-argument", src, param, text, found, why, improvement), true
}

// --- request-without-timeout ---------------------------------------------------

var requestMethods = map[string]bool{
	"get": true, "post": true, "put": true, "patch": true, "delete": true, "head": true, "options": true, "request": true,
}

func checkRequestWithoutTimeout(src *source, call *sitter.Node) (review.Finding, bool) {
	fn := call.ChildByFieldName("function")
	if fn == nil || fn.Type() != "attribute" {
		return review.Finding{}, false
	}
	obj, attr := fn.ChildByFieldName("object"), fn.ChildByFieldName("attribute")
	if obj == nil || attr == nil || src.text(obj) != "requests" || !requestMethods[src.text(attr)] {
		return review.Finding{}, false
	}
	args := call.ChildByFieldName("arguments")
	if args == nil {
		return review.Finding{}, false
	}
	for i := 0; i < int(args.NamedChildCount()); i++ {
		arg := args.NamedChild(i)
		switch arg.Type() {
		case "dictionary_splat":
			return review.Finding{}, false // **options might hold the timeout; don't guess
		case "keyword_argument":
			if name := arg.ChildByFieldName("name"); name != nil && src.text(name) == "timeout" {
				return review.Finding{}, false
			}
		}
	}

	text := src.text(call)
	found := fmt.Sprintf("`%s` has no `timeout`, so it can wait forever.", text)
	why := "The requests library never times out by default. If the server stops answering, this call hangs, and with it the worker, request or script that made it."
	improvement := "Pass a timeout in seconds, for example `requests.get(url, timeout=10)`, or a `(connect, read)` pair."
	return newFinding("request-without-timeout", src, call, text, found, why, improvement), true
}

// --- high-complexity -----------------------------------------------------------

func checkHighComplexity(src *source, fn *sitter.Node, threshold int) (review.Finding, bool) {
	body := fn.ChildByFieldName("body")
	if body == nil {
		return review.Finding{}, false
	}

	complexity := 1
	walk(body, func(n *sitter.Node) {
		switch n.Type() {
		case "if_statement", "elif_clause", "for_statement", "while_statement", "except_clause",
			"conditional_expression", "case_clause", "boolean_operator":
			complexity++
		}
	})
	if complexity <= threshold {
		return review.Finding{}, false
	}

	name := "This function"
	if id := fn.ChildByFieldName("name"); id != nil {
		name = fmt.Sprintf("`%s`", src.text(id))
	}
	evidence := fmt.Sprintf("Cyclomatic complexity of %s is %d (threshold %d).", name, complexity, threshold)
	found := fmt.Sprintf("%s has %d independent paths through it.", name, complexity)
	why := "Each extra branch is another case to test and hold in your head. Functions this size tend to hide bugs in their rarely exercised paths."
	improvement := "Extract validation, branching or result handling into smaller, named functions."
	return newFinding("high-complexity", src, fn, evidence, found, why, improvement), true
}
