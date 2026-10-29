package tsast

import (
	"fmt"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/review"
)

// --- loose-equality ----------------------------------------------------------

func checkLooseEquality(src *source, node *sitter.Node) (review.Finding, bool) {
	op := node.ChildByFieldName("operator")
	if op == nil || (op.Type() != "==" && op.Type() != "!=") {
		return review.Finding{}, false
	}

	text := src.text(node)
	found := fmt.Sprintf("`%s` uses loose equality (`%s`), which converts the operands' types before comparing.", text, op.Type())
	why := "Loose equality has surprising coercion rules (`0 == false`, `\"\" == 0`, `null == undefined`), so it can match or fail to match in ways that don't reflect the actual values."
	improvement := fmt.Sprintf("Use strict equality instead: replace `%s` with `%s`.", op.Type(), strictForm(op.Type()))

	return newFinding("loose-equality", src, node, text, found, why, improvement), true
}

func strictForm(op string) string {
	if op == "==" {
		return "==="
	}
	return "!=="
}

// --- any-type ------------------------------------------------------------

func checkAnyType(src *source, node *sitter.Node) (review.Finding, bool) {
	if src.text(node) != "any" {
		return review.Finding{}, false
	}

	// type_annotation -> predefined_type; its parent names what this types.
	what := "This"
	if ann := node.Parent(); ann != nil {
		if owner := ann.Parent(); owner != nil {
			switch owner.Type() {
			case "required_parameter", "optional_parameter":
				what = "This parameter"
			case "function_declaration", "method_definition", "arrow_function":
				what = "This function's return type"
			case "variable_declarator", "public_field_definition":
				what = "This value"
			}
		}
	}

	found := fmt.Sprintf("%s is typed `any`, which disables type checking for it.", what)
	why := "Anything assigned to or read from an `any` value skips type checking entirely, so mistakes that TypeScript would normally catch at compile time (wrong property name, wrong argument type) surface at runtime instead, if at all."
	improvement := "Give it the real type, or `unknown` if the type genuinely isn't known yet — `unknown` still forces a check before use."

	return newFinding("any-type", src, node, src.text(node), found, why, improvement), true
}

// --- console-log-left-in --------------------------------------------------

func checkConsoleLog(src *source, call *sitter.Node) (review.Finding, bool) {
	fn := call.ChildByFieldName("function")
	if fn == nil || fn.Type() != "member_expression" {
		return review.Finding{}, false
	}
	obj, prop := fn.ChildByFieldName("object"), fn.ChildByFieldName("property")
	if obj == nil || prop == nil || src.text(obj) != "console" || src.text(prop) != "log" {
		return review.Finding{}, false
	}

	text := src.text(call)
	found := fmt.Sprintf("`%s` looks like debug output left in the code.", text)
	why := "Console output left behind clutters production logs (or a browser's console) with information that was only useful while writing this change."
	improvement := "Remove it, or replace it with a real logger call if the output needs to stay."

	return newFinding("console-log-left-in", src, call, text, found, why, improvement), true
}

// --- unhandled-promise ------------------------------------------------------

func checkUnhandledPromise(src *source, stmt *sitter.Node) (review.Finding, bool) {
	if stmt.NamedChildCount() != 1 {
		return review.Finding{}, false
	}
	call := stmt.NamedChild(0)
	if call.Type() != "call_expression" {
		return review.Finding{}, false
	}
	fn := call.ChildByFieldName("function")
	if fn == nil || fn.Type() != "member_expression" {
		return review.Finding{}, false
	}
	prop := fn.ChildByFieldName("property")
	if prop == nil || src.text(prop) != "then" {
		return review.Finding{}, false
	}

	text := src.text(stmt)
	found := fmt.Sprintf("`%s` ends in `.then()` with no `.catch()`, and its result is not awaited, returned or assigned.", text)
	why := "If the promise rejects, nothing handles it: in Node this can crash the process, and in a browser it's a silent unhandled rejection."
	improvement := "Add a `.catch(...)` to the chain, or `await` it inside a `try`/`catch`."

	return newFinding("unhandled-promise", src, stmt, text, found, why, improvement), true
}

// --- fetch-without-abort-signal ----------------------------------------------

func checkFetchWithoutSignal(src *source, call *sitter.Node) (review.Finding, bool) {
	fn := call.ChildByFieldName("function")
	if fn == nil || fn.Type() != "identifier" || src.text(fn) != "fetch" {
		return review.Finding{}, false
	}
	args := call.ChildByFieldName("arguments")
	if args == nil {
		return review.Finding{}, false
	}

	if args.NamedChildCount() < 2 {
		return fetchFinding(src, call)
	}
	opts := args.NamedChild(1)
	if opts.Type() != "object" {
		return review.Finding{}, false // not a literal we can inspect; don't guess
	}
	for i := 0; i < int(opts.NamedChildCount()); i++ {
		pair := opts.NamedChild(i)
		if pair.Type() == "pair" {
			if key := pair.ChildByFieldName("key"); key != nil && src.text(key) == "signal" {
				return review.Finding{}, false
			}
		}
	}
	return fetchFinding(src, call)
}

func fetchFinding(src *source, call *sitter.Node) (review.Finding, bool) {
	text := src.text(call)
	found := fmt.Sprintf("`%s` has no AbortSignal, so the request cannot be cancelled.", text)
	why := "If the caller gives up (navigates away, times out, retries), this request keeps running on the network and its eventual response is wasted work."
	improvement := "Pass an AbortController's signal: `fetch(url, { signal: controller.signal })`, and call `controller.abort()` when the request should stop."

	return newFinding("fetch-without-abort-signal", src, call, text, found, why, improvement), true
}

// --- high-complexity ----------------------------------------------------------

func checkHighComplexity(src *source, fn *sitter.Node, threshold int) (review.Finding, bool) {
	body := fn.ChildByFieldName("body")
	if body == nil {
		return review.Finding{}, false
	}

	complexity := 1
	walk(body, func(n *sitter.Node) {
		switch n.Type() {
		case "if_statement", "for_statement", "for_in_statement", "while_statement",
			"do_statement", "switch_case", "ternary_expression":
			complexity++
		case "binary_expression":
			if op := n.ChildByFieldName("operator"); op != nil && (op.Type() == "&&" || op.Type() == "||") {
				complexity++
			}
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
	improvement := "Extract validation, branching or response handling into smaller, named functions."

	return newFinding("high-complexity", src, fn, evidence, found, why, improvement), true
}
