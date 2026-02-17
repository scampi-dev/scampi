// SPDX-License-Identifier: GPL-3.0-only

package star

import (
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"

	"godoit.dev/doit/spec"
)

func posFromSpan(s spec.SourceSpan) syntax.Position {
	return syntax.MakePosition(&s.Filename, int32(s.StartLine), int32(s.StartCol))
}

func posToSpan(pos syntax.Position) spec.SourceSpan {
	return spec.SourceSpan{
		Filename:  pos.Filename(),
		StartLine: int(pos.Line),
		StartCol:  int(pos.Col),
		EndLine:   int(pos.Line),
		EndCol:    int(pos.Col),
	}
}

// callSpan returns a SourceSpan for the call site of the current builtin.
// It looks at the second-to-last entry in the call stack (the caller).
func callSpan(thread *starlark.Thread) spec.SourceSpan {
	stack := thread.CallStack()
	if len(stack) < 2 {
		return spec.SourceSpan{}
	}
	return posToSpan(stack[len(stack)-2].Pos)
}

// kwargsFieldSpans produces a FieldSpan map by walking the Starlark AST to
// find the source position of each kwarg value expression. Falls back to the
// call-site position when the AST is unavailable or a kwarg isn't found.
func kwargsFieldSpans(thread *starlark.Thread, names ...string) map[string]spec.FieldSpan {
	callerPos := callerPosition(thread)
	fields := make(map[string]spec.FieldSpan, len(names))

	call := findCallFromThread(thread, callerPos)

	for _, name := range names {
		if call != nil {
			if vs, ok := kwargValueSpan(call, name); ok {
				fields[name] = spec.FieldSpan{Value: vs}
				continue
			}
		}
		fields[name] = spec.FieldSpan{Value: posToSpan(callerPos)}
	}
	return fields
}

// callerPosition returns the syntax.Position of the builtin's call site.
func callerPosition(thread *starlark.Thread) syntax.Position {
	stack := thread.CallStack()
	if len(stack) < 2 {
		return syntax.Position{}
	}
	return stack[len(stack)-2].Pos
}

// findCallFromThread locates the CallExpr in the AST that corresponds to
// the current builtin invocation.
func findCallFromThread(thread *starlark.Thread, pos syntax.Position) *syntax.CallExpr {
	if !pos.IsValid() {
		return nil
	}
	c := threadCollector(thread)
	f := c.AST(pos.Filename())
	if f == nil {
		return nil
	}
	return findCallExpr(f, pos)
}

// findCallExpr walks a parsed file to find the CallExpr whose Lparen matches
// the given position. The Starlark compiler records Lparen as the position for
// CALL instructions, so thread.CallStack() positions correspond to Lparen.
func findCallExpr(file *syntax.File, pos syntax.Position) *syntax.CallExpr {
	var found *syntax.CallExpr
	syntax.Walk(file, func(n syntax.Node) bool {
		if found != nil {
			return false
		}
		call, ok := n.(*syntax.CallExpr)
		if !ok {
			return true
		}
		if call.Lparen.Line == pos.Line && call.Lparen.Col == pos.Col {
			found = call
			return false
		}
		return true
	})
	return found
}

// firstArgSpan extracts the source span of the first positional argument
// from the current builtin's call site. Falls back to the call-site span.
func firstArgSpan(thread *starlark.Thread) spec.SourceSpan {
	pos := callerPosition(thread)
	call := findCallFromThread(thread, pos)
	if call != nil {
		for _, arg := range call.Args {
			if _, ok := arg.(*syntax.BinaryExpr); ok {
				continue // skip kwargs
			}
			start, end := arg.Span()
			return spec.SourceSpan{
				Filename:  start.Filename(),
				StartLine: int(start.Line),
				StartCol:  int(start.Col),
				EndLine:   int(end.Line),
				EndCol:    int(end.Col),
			}
		}
	}
	return posToSpan(pos)
}

// kwargKeySpan extracts the source span for a named kwarg's key identifier
// from a CallExpr. Used for diagnostics about the kwarg name itself (e.g.
// unknown keyword argument).
func kwargKeySpan(call *syntax.CallExpr, name string) (spec.SourceSpan, bool) {
	for _, arg := range call.Args {
		bin, ok := arg.(*syntax.BinaryExpr)
		if !ok || bin.Op != syntax.EQ {
			continue
		}
		ident, ok := bin.X.(*syntax.Ident)
		if !ok || ident.Name != name {
			continue
		}
		start, end := ident.Span()
		return spec.SourceSpan{
			Filename:  start.Filename(),
			StartLine: int(start.Line),
			StartCol:  int(start.Col),
			EndLine:   int(end.Line),
			EndCol:    int(end.Col),
		}, true
	}
	return spec.SourceSpan{}, false
}

// kwargValueSpan extracts the source span for a named kwarg's value
// expression from a CallExpr.
func kwargValueSpan(call *syntax.CallExpr, name string) (spec.SourceSpan, bool) {
	for _, arg := range call.Args {
		bin, ok := arg.(*syntax.BinaryExpr)
		if !ok || bin.Op != syntax.EQ {
			continue
		}
		ident, ok := bin.X.(*syntax.Ident)
		if !ok || ident.Name != name {
			continue
		}
		start, end := bin.Y.Span()
		return spec.SourceSpan{
			Filename:  start.Filename(),
			StartLine: int(start.Line),
			StartCol:  int(start.Col),
			EndLine:   int(end.Line),
			EndCol:    int(end.Col),
		}, true
	}
	return spec.SourceSpan{}, false
}

// refinePoisonSpan attempts to replace the LPAREN-level span on a
// PoisonValueError with the precise span of the nested declaration call
// (e.g. secrets(...) inside a template's data dict).
func refinePoisonSpan(pe *PoisonValueError, c *Collector, callSite spec.SourceSpan) {
	if c == nil {
		return
	}
	f := c.AST(callSite.Filename)
	if f == nil {
		return
	}
	call := findCallExpr(f, posFromSpan(callSite))
	if call == nil {
		return
	}
	if s, ok := nestedCallSpan(call, pe.FuncName); ok {
		pe.Source = s
	}
}

// nestedCallSpan walks the argument subtree of a CallExpr looking for a
// nested call to the named function. Used to refine poison value errors so
// they point at the offending declaration call rather than the enclosing step.
func nestedCallSpan(call *syntax.CallExpr, funcName string) (spec.SourceSpan, bool) {
	var found *syntax.CallExpr
	for _, arg := range call.Args {
		syntax.Walk(arg, func(n syntax.Node) bool {
			if n == nil || found != nil {
				return false
			}
			c, ok := n.(*syntax.CallExpr)
			if !ok {
				return true
			}
			ident, ok := c.Fn.(*syntax.Ident)
			if !ok || ident.Name != funcName {
				return true
			}
			found = c
			return false
		})
		if found != nil {
			break
		}
	}
	if found == nil {
		return spec.SourceSpan{}, false
	}
	start, end := found.Span()
	return spec.SourceSpan{
		Filename:  start.Filename(),
		StartLine: int(start.Line),
		StartCol:  int(start.Col),
		EndLine:   int(end.Line),
		EndCol:    int(end.Col),
	}, true
}
