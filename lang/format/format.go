// SPDX-License-Identifier: GPL-3.0-only

// Package format implements the canonical scampi source formatter.
// It parses source, preserves comments, and re-emits the code in
// canonical style (2-space indent, normalized spacing).
package format

import (
	"bytes"
	"sort"
	"strings"

	"scampi.dev/scampi/lang/ast"
	"scampi.dev/scampi/lang/lex"
	"scampi.dev/scampi/lang/parse"
	"scampi.dev/scampi/lang/token"
)

// Format parses src and returns canonically formatted source.
// Returns the original source unchanged if parsing fails.
func Format(src []byte) ([]byte, error) {
	l := lex.New("<fmt>", src)
	p := parse.New(l)
	f := p.Parse()
	if errs := l.Errors(); len(errs) > 0 {
		return src, nil
	}
	if errs := p.Errors(); len(errs) > 0 {
		return src, nil
	}

	pr := &printer{
		src:      src,
		comments: l.Comments(),
		buf:      &bytes.Buffer{},
	}
	pr.file(f)
	return pr.buf.Bytes(), nil
}

type printer struct {
	src      []byte
	comments []lex.Comment
	buf      *bytes.Buffer
	indent   int
}

// Indent Helpers
// -----------------------------------------------------------------------------

func (p *printer) in()  { p.indent++ }
func (p *printer) out() { p.indent-- }

func (p *printer) writeIndent() {
	for range p.indent {
		_, _ = p.buf.WriteString("  ")
	}
}

func (p *printer) write(s string) {
	_, _ = p.buf.WriteString(s)
}

func (p *printer) writeln(s string) {
	p.writeIndent()
	_, _ = p.buf.WriteString(s)
	_ = p.buf.WriteByte('\n')
}

func (p *printer) newline() {
	_ = p.buf.WriteByte('\n')
}

// Comment Interleaving
// -----------------------------------------------------------------------------

// emitCommentsBefore writes any comments whose start position falls
// before `pos`, preserving their original indentation relative to
// the current indent level.
func (p *printer) emitCommentsBefore(pos uint32) {
	for len(p.comments) > 0 && p.comments[0].Start < pos {
		c := p.comments[0]
		p.comments = p.comments[1:]
		p.writeIndent()
		p.write(c.Text)
		p.newline()
		// Preserve blank line between this comment and whatever follows
		// (the next comment or the node at `pos`).
		nextPos := pos
		if len(p.comments) > 0 && p.comments[0].Start < pos {
			nextPos = p.comments[0].Start
		}
		if p.hasBlankLineBetween(c.End, nextPos) {
			p.newline()
		}
	}
}

// emitTrailingComments writes any remaining comments after the last node.
func (p *printer) emitTrailingComments() {
	for _, c := range p.comments {
		p.write(c.Text)
		p.newline()
	}
	p.comments = nil
}

// File
// -----------------------------------------------------------------------------

func (p *printer) file(f *ast.File) {
	if f.Module != nil {
		p.emitCommentsBefore(f.Module.SrcSpan.Start)
		p.writeln("module " + f.Module.Name.Name)
	}

	if len(f.Imports) > 0 {
		p.newline()
		for _, imp := range f.Imports {
			p.emitCommentsBefore(imp.SrcSpan.Start)
			p.writeln("import \"" + imp.Path + "\"")
		}
	}

	// Merge decls and stmts by source position so interleaved
	// items emit in the correct order.
	type item struct {
		pos  uint32
		decl ast.Decl
		stmt ast.Stmt
	}
	var items []item
	for _, d := range f.Decls {
		items = append(items, item{pos: d.Span().Start, decl: d})
	}
	for _, s := range f.Stmts {
		items = append(items, item{pos: s.Span().Start, stmt: s})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].pos < items[j].pos })

	var prevEnd uint32
	if len(f.Imports) > 0 {
		prevEnd = f.Imports[len(f.Imports)-1].SrcSpan.End
	} else if f.Module != nil {
		prevEnd = f.Module.SrcSpan.End
	}
	for _, it := range items {
		var itemStart uint32
		if it.decl != nil {
			itemStart = it.decl.Span().Start
		} else {
			itemStart = it.stmt.Span().Start
		}
		// Check for blank line using the earliest relevant position:
		// the first pending comment before this item, or the item itself.
		target := itemStart
		if len(p.comments) > 0 && p.comments[0].Start < itemStart {
			target = p.comments[0].Start
		}
		if p.hasBlankLineBetween(prevEnd, target) || prevEnd == 0 {
			p.newline()
		}
		if it.decl != nil {
			p.emitCommentsBefore(it.decl.Span().Start)
			p.decl(it.decl)
			prevEnd = it.decl.Span().End
		} else {
			p.emitCommentsBefore(it.stmt.Span().Start)
			p.stmt(it.stmt)
			prevEnd = it.stmt.Span().End
		}
	}

	p.emitTrailingComments()
}

// Declarations
// -----------------------------------------------------------------------------

func (p *printer) decl(d ast.Decl) {
	switch d := d.(type) {
	case *ast.LetDecl:
		p.letDecl(d)
	case *ast.FuncDecl:
		p.funcDecl(d)
	case *ast.DeclDecl:
		p.declDecl(d)
	case *ast.TypeDecl:
		p.typeDecl(d)
	case *ast.EnumDecl:
		p.enumDecl(d)
	case *ast.AttrTypeDecl:
		p.attrTypeDecl(d)
	}
}

func (p *printer) pubPrefix(public bool) {
	if public {
		p.write("pub ")
	}
}

func (p *printer) letDecl(d *ast.LetDecl) {
	p.writeIndent()
	p.pubPrefix(d.Public)
	p.write("let " + d.Name.Name + " = ")
	p.expr(d.Value)
	p.newline()
}

func (p *printer) funcDecl(d *ast.FuncDecl) {
	p.writeIndent()
	p.pubPrefix(d.Public)
	p.write("func " + d.Name.Name + "(")
	p.fieldList(d.Params, d.SrcSpan)
	p.write(")")
	if d.Ret != nil {
		p.write(" ")
		p.typeExpr(d.Ret)
	}
	if d.Body != nil {
		p.write(" ")
		p.block(d.Body)
	}
	p.newline()
}

func (p *printer) declDecl(d *ast.DeclDecl) {
	p.writeIndent()
	p.pubPrefix(d.Public)
	p.write("decl ")
	p.dottedName(d.Name)
	p.write("(")
	p.fieldList(d.Params, d.SrcSpan)
	p.write(")")
	if d.Ret != nil {
		p.write(" ")
		p.typeExpr(d.Ret)
	}
	if d.Body != nil {
		p.write(" ")
		p.block(d.Body)
	}
	p.newline()
}

func (p *printer) typeDecl(d *ast.TypeDecl) {
	p.writeIndent()
	p.pubPrefix(d.Public)
	if d.Fields == nil {
		p.write("type " + d.Name.Name)
		p.newline()
		return
	}
	p.write("type " + d.Name.Name + " {")
	p.newline()
	p.in()
	maxName := maxFieldNameLen(d.Fields)
	for _, f := range d.Fields {
		p.emitCommentsBefore(f.SrcSpan.Start)
		for _, attr := range f.Attributes {
			p.writeIndent()
			p.attribute(attr)
			p.newline()
		}
		p.field(f, maxName)
	}
	p.out()
	p.writeln("}")
}

func (p *printer) enumDecl(d *ast.EnumDecl) {
	p.writeIndent()
	p.pubPrefix(d.Public)
	p.write("enum " + d.Name.Name + " { ")
	for i, v := range d.Variants {
		if i > 0 {
			p.write(", ")
		}
		p.write(v.Name)
	}
	p.write(" }")
	p.newline()
}

func (p *printer) attrTypeDecl(d *ast.AttrTypeDecl) {
	p.writeIndent()
	if len(d.Fields) == 0 {
		p.write("type @" + d.Name.Name + " {}")
		p.newline()
		return
	}
	p.write("type @" + d.Name.Name + " {")
	p.newline()
	p.in()
	maxName := maxFieldNameLen(d.Fields)
	for _, f := range d.Fields {
		for _, attr := range f.Attributes {
			p.writeIndent()
			p.attribute(attr)
			p.newline()
		}
		p.field(f, maxName)
	}
	p.out()
	p.writeln("}")
}

// Fields
// -----------------------------------------------------------------------------

func (p *printer) fieldList(fields []*ast.Field, _ token.Span) {
	if len(fields) == 0 {
		return
	}
	// Single-line if the params themselves were on one line in the
	// original source. Use the span from first to last field, not
	// the parent's full span (which includes the body).
	paramsSpan := token.Span{Start: fields[0].SrcSpan.Start, End: fields[len(fields)-1].SrcSpan.End}
	if !p.spanContainsNewline(paramsSpan) {
		for i, f := range fields {
			if i > 0 {
				p.write(", ")
			}
			p.fieldInline(f)
		}
		return
	}
	maxName := maxFieldNameLen(fields)
	p.newline()
	p.in()
	for _, f := range fields {
		p.emitCommentsBefore(f.SrcSpan.Start)
		for _, attr := range f.Attributes {
			p.writeIndent()
			p.attribute(attr)
			p.newline()
		}
		p.field(f, maxName)
	}
	p.out()
}

func (p *printer) field(f *ast.Field, align int) {
	p.writeIndent()
	name := f.Name.Name
	padding := strings.Repeat(" ", align-len(name))
	p.write(name + ":" + padding + " ")
	p.typeExpr(f.Type)
	if f.Default != nil {
		p.write(" = ")
		p.expr(f.Default)
	}
	p.newline()
}

func maxFieldNameLen(fields []*ast.Field) int {
	longest := 0
	for _, f := range fields {
		if len(f.Name.Name) > longest {
			longest = len(f.Name.Name)
		}
	}
	return longest
}

func maxFieldInitNameLen(fields []*ast.FieldInit) int {
	longest := 0
	for _, f := range fields {
		if len(f.Name.Name) > longest {
			longest = len(f.Name.Name)
		}
	}
	return longest
}

func maxCallArgNameLen(args []*ast.CallArg) int {
	longest := 0
	for _, a := range args {
		if a.Name != nil && len(a.Name.Name) > longest {
			longest = len(a.Name.Name)
		}
	}
	return longest
}

func (p *printer) fieldInline(f *ast.Field) {
	for _, attr := range f.Attributes {
		p.attribute(attr)
		p.write(" ")
	}
	p.write(f.Name.Name + ": ")
	p.typeExpr(f.Type)
	if f.Default != nil {
		p.write(" = ")
		p.expr(f.Default)
	}
}

func (p *printer) attribute(a *ast.Attribute) {
	p.write("@")
	p.dottedName(a.Name)
	if len(a.Positionals) == 0 && len(a.Named) == 0 {
		return
	}
	p.write("(")
	first := true
	for _, pos := range a.Positionals {
		if !first {
			p.write(", ")
		}
		p.expr(pos)
		first = false
	}
	for _, na := range a.Named {
		if !first {
			p.write(", ")
		}
		p.write(na.Name.Name + "=")
		p.expr(na.Value)
		first = false
	}
	p.write(")")
}

// Type Expressions
// -----------------------------------------------------------------------------

func (p *printer) typeExpr(te ast.TypeExpr) {
	switch t := te.(type) {
	case *ast.NamedType:
		p.dottedName(t.Name)
	case *ast.GenericType:
		p.write(t.Name.Name + "[")
		for i, arg := range t.Args {
			if i > 0 {
				p.write(", ")
			}
			p.typeExpr(arg)
		}
		p.write("]")
	case *ast.OptionalType:
		p.typeExpr(t.Inner)
		p.write("?")
	}
}

// Statements
// -----------------------------------------------------------------------------

func (p *printer) stmt(s ast.Stmt) {
	switch s := s.(type) {
	case *ast.LetStmt:
		p.letDecl(s.Decl)
	case *ast.ExprStmt:
		p.writeIndent()
		p.expr(s.Expr)
		p.newline()
	case *ast.AssignStmt:
		p.writeIndent()
		p.expr(s.Target)
		p.write(" = ")
		p.expr(s.Value)
		p.newline()
	case *ast.ForStmt:
		p.writeIndent()
		p.write("for " + s.Var.Name + " in ")
		p.expr(s.Iter)
		p.write(" ")
		p.block(s.Body)
		p.newline()
	case *ast.IfStmt:
		p.writeIndent()
		p.write("if ")
		p.expr(s.Cond)
		p.write(" ")
		p.block(s.Then)
		if s.Else != nil {
			p.write(" else ")
			p.block(s.Else)
		}
		p.newline()
	case *ast.ReturnStmt:
		p.writeIndent()
		if s.Value != nil {
			p.write("return ")
			p.expr(s.Value)
		} else {
			p.write("return")
		}
		p.newline()
	}
}

func (p *printer) block(b *ast.Block) {
	p.write("{")
	p.newline()
	p.in()
	p.stmtSequence(b.Stmts)
	p.emitCommentsBefore(b.SrcSpan.End)
	p.out()
	p.writeIndent()
	p.write("}")
}

// stmtSequence emits a slice of statements, detecting column groups
// of consecutive one-line struct-lit expressions and aligning them.
func (p *printer) stmtSequence(stmts []ast.Stmt) {
	var prevEnd uint32
	i := 0
	for i < len(stmts) {
		s := stmts[i]
		target := s.Span().Start
		if len(p.comments) > 0 && p.comments[0].Start < target {
			target = p.comments[0].Start
		}
		if i > 0 && p.hasBlankLineBetween(prevEnd, target) {
			p.newline()
		}

		// Try to form a column group starting at i.
		groupEnd := p.findColumnGroup(stmts, i)
		if groupEnd > i+1 {
			p.emitColumnGroup(stmts[i:groupEnd])
			prevEnd = stmts[groupEnd-1].Span().End
			i = groupEnd
			continue
		}

		p.emitCommentsBefore(s.Span().Start)
		p.stmt(s)
		prevEnd = s.Span().End
		i++
	}
}

// Expressions
// -----------------------------------------------------------------------------

func (p *printer) expr(e ast.Expr) {
	switch e := e.(type) {
	case *ast.Ident:
		p.write(e.Name)
	case *ast.IntLit:
		p.write(e.Raw)
	case *ast.BoolLit:
		if e.Value {
			p.write("true")
		} else {
			p.write("false")
		}
	case *ast.NoneLit:
		p.write("none")
	case *ast.SelfLit:
		p.write("self")
	case *ast.StringLit:
		p.stringLit(e)
	case *ast.ListLit:
		p.listLit(e)
	case *ast.MapLit:
		p.mapLit(e)
	case *ast.StructLit:
		p.structLit(e)
	case *ast.BlockExpr:
		p.blockExpr(e)
	case *ast.CallExpr:
		p.callExpr(e)
	case *ast.SelectorExpr:
		p.expr(e.X)
		p.write("." + e.Sel.Name)
	case *ast.IndexExpr:
		p.expr(e.X)
		p.write("[")
		p.expr(e.Index)
		p.write("]")
	case *ast.ParenExpr:
		p.write("(")
		p.expr(e.Inner)
		p.write(")")
	case *ast.BinaryExpr:
		p.expr(e.Left)
		p.write(" " + opSymbol(e.Op) + " ")
		p.expr(e.Right)
	case *ast.UnaryExpr:
		p.write(opSymbol(e.Op))
		p.expr(e.X)
	case *ast.IfExpr:
		p.write("if ")
		p.expr(e.Cond)
		p.write(" { ")
		p.expr(e.Then)
		p.write(" } else { ")
		p.expr(e.Else)
		p.write(" }")
	case *ast.ListComp:
		p.write("[")
		p.expr(e.Expr)
		p.write(" for " + e.Var.Name + " in ")
		p.expr(e.Iter)
		if e.Cond != nil {
			p.write(" if ")
			p.expr(e.Cond)
		}
		p.write("]")
	case *ast.MapComp:
		p.write("{")
		p.expr(e.Key)
		p.write(": ")
		p.expr(e.Value)
		vars := make([]string, len(e.Vars))
		for i, v := range e.Vars {
			vars[i] = v.Name
		}
		p.write(" for " + strings.Join(vars, ", ") + " in ")
		p.expr(e.Iter)
		if e.Cond != nil {
			p.write(" if ")
			p.expr(e.Cond)
		}
		p.write("}")
	case *ast.DottedName:
		p.dottedName(e)
	}
}

func (p *printer) stringLit(s *ast.StringLit) {
	delim := "\""
	if s.MultiLine {
		delim = "`"
	}
	p.write(delim)
	for _, part := range s.Parts {
		switch part := part.(type) {
		case *ast.StringText:
			p.write(part.Raw)
		case *ast.StringInterp:
			p.write("${")
			p.expr(part.Expr)
			p.write("}")
		}
	}
	p.write(delim)
}

func (p *printer) listLit(l *ast.ListLit) {
	if len(l.Items) == 0 {
		p.write("[]")
		return
	}
	// Single-line if short
	if p.isShortList(l) {
		p.write("[")
		for i, item := range l.Items {
			if i > 0 {
				p.write(", ")
			}
			p.expr(item)
		}
		p.write("]")
		return
	}
	p.write("[")
	p.newline()
	p.in()
	p.listItemsAligned(l.Items)
	p.emitCommentsBefore(l.SrcSpan.End)
	p.out()
	p.writeIndent()
	p.write("]")
}

func (p *printer) mapLit(m *ast.MapLit) {
	if len(m.Entries) == 0 {
		p.write("{}")
		return
	}
	if !p.spanContainsNewline(m.SrcSpan) {
		p.write("{")
		for i, e := range m.Entries {
			if i > 0 {
				p.write(", ")
			}
			p.expr(e.Key)
			p.write(": ")
			p.expr(e.Value)
		}
		p.write("}")
		return
	}
	p.write("{")
	p.newline()
	p.in()
	for _, e := range m.Entries {
		p.writeIndent()
		p.expr(e.Key)
		p.write(": ")
		p.expr(e.Value)
		p.write(",")
		p.newline()
	}
	p.out()
	p.writeIndent()
	p.write("}")
}

func (p *printer) structLit(s *ast.StructLit) {
	if s.Type != nil {
		p.typeExpr(s.Type)
		p.write(" ")
	}
	if len(s.Fields) == 0 && len(s.Body) == 0 {
		p.write("{}")
		return
	}
	// Single-line if the original source had everything on one line.
	// User forces multi-line by putting } on its own line.
	if len(s.Body) == 0 && !p.spanContainsNewline(s.SrcSpan) {
		p.write("{ ")
		for i, fi := range s.Fields {
			if i > 0 {
				p.write(", ")
			}
			p.write(fi.Name.Name + " = ")
			p.expr(fi.Value)
		}
		p.write(" }")
		return
	}
	p.write("{")
	p.newline()
	p.in()
	maxInit := maxFieldInitNameLen(s.Fields)
	for _, fi := range s.Fields {
		p.emitCommentsBefore(fi.SrcSpan.Start)
		p.writeIndent()
		name := fi.Name.Name
		padding := strings.Repeat(" ", maxInit-len(name))
		p.write(name + padding + " = ")
		p.expr(fi.Value)
		p.newline()
	}
	for _, st := range s.Body {
		p.emitCommentsBefore(st.Span().Start)
		p.stmt(st)
	}
	p.out()
	p.writeIndent()
	p.write("}")
}

func (p *printer) blockExpr(b *ast.BlockExpr) {
	p.expr(b.Target)
	p.write(" ")
	p.block(b.Body)
}

func (p *printer) callExpr(c *ast.CallExpr) {
	p.expr(c.Fn)
	if !p.spanContainsNewline(c.SrcSpan) {
		p.write("(")
		for i, arg := range c.Args {
			if i > 0 {
				p.write(", ")
			}
			if arg.Name != nil {
				p.write(arg.Name.Name + " = ")
			}
			p.expr(arg.Value)
		}
		p.write(")")
		return
	}
	p.write("(")
	p.newline()
	p.in()
	maxArg := maxCallArgNameLen(c.Args)
	for _, arg := range c.Args {
		p.writeIndent()
		if arg.Name != nil {
			name := arg.Name.Name
			padding := strings.Repeat(" ", maxArg-len(name))
			p.write(name + padding + " = ")
		}
		p.expr(arg.Value)
		p.write(",")
		p.newline()
	}
	p.out()
	p.writeIndent()
	p.write(")")
}

func (p *printer) dottedName(dn *ast.DottedName) {
	for i, part := range dn.Parts {
		if i > 0 {
			p.write(".")
		}
		p.write(part.Name)
	}
}

// Helpers
// -----------------------------------------------------------------------------

var opSymbols = map[token.Kind]string{
	token.Plus:    "+",
	token.Minus:   "-",
	token.Star:    "*",
	token.Slash:   "/",
	token.Percent: "%",
	token.Eq:      "==",
	token.Neq:     "!=",
	token.Lt:      "<",
	token.Gt:      ">",
	token.Leq:     "<=",
	token.Geq:     ">=",
	token.And:     "&&",
	token.Or:      "||",
	token.Not:     "!",
	token.In:      "in",
}

func opSymbol(k token.Kind) string {
	if s, ok := opSymbols[k]; ok {
		return s
	}
	return k.String()
}

func (p *printer) isShortList(l *ast.ListLit) bool {
	return !p.spanContainsNewline(l.SrcSpan)
}

func (p *printer) spanContainsNewline(s token.Span) bool {
	return p.spanContainsNewlineRange(s.Start, s.End)
}

func (p *printer) spanContainsNewlineRange(start, end uint32) bool {
	if int(end) > len(p.src) {
		end = uint32(len(p.src))
	}
	return bytes.ContainsRune(p.src[start:end], '\n')
}

// hasBlankLineBetween reports whether the original source has at least
// one blank line between two byte positions. Scans from `from` forward
// to `to`, looking for an empty line (a line containing only whitespace).
func (p *printer) hasBlankLineBetween(from, to uint32) bool {
	if int(to) > len(p.src) {
		to = uint32(len(p.src))
	}
	// The parser's ASI may set span End past the line-ending \n,
	// so back up to include it in the gap.
	if from > 0 {
		from--
	}
	if from >= to {
		return false
	}
	// Strip non-newline whitespace so indentation doesn't mask
	// blank lines. A blank line is two consecutive \n after stripping.
	var stripped []byte
	for _, b := range p.src[from:to] {
		if b == '\n' {
			stripped = append(stripped, b)
		} else if b != ' ' && b != '\t' && b != '\r' {
			stripped = append(stripped, b)
		}
	}
	return bytes.Contains(stripped, []byte("\n\n"))
}

// Column alignment for consecutive one-line blocks
// -----------------------------------------------------------------------------

// oneLineStructLit returns the StructLit if stmt is an ExprStmt wrapping
// a one-line typed struct literal, or nil otherwise.
func (p *printer) oneLineStructLit(s ast.Stmt) *ast.StructLit {
	es, ok := s.(*ast.ExprStmt)
	if !ok {
		return nil
	}
	sl, ok := es.Expr.(*ast.StructLit)
	if !ok || len(sl.Fields) == 0 || len(sl.Body) > 0 {
		return nil
	}
	if p.spanContainsNewline(sl.SrcSpan) {
		return nil
	}
	return sl
}

// columnFieldNames returns the field names of a struct lit.
func columnFieldNames(sl *ast.StructLit) []string {
	names := make([]string, len(sl.Fields))
	for i, fi := range sl.Fields {
		names[i] = fi.Name.Name
	}
	return names
}

// columnTypeName returns the type prefix for grouping (e.g. "rest.request").
func columnTypeName(sl *ast.StructLit) string {
	nt, ok := sl.Type.(*ast.NamedType)
	if !ok {
		return ""
	}
	var b strings.Builder
	for _, part := range nt.Name.Parts {
		if b.Len() > 0 {
			b.WriteByte('.')
		}
		b.WriteString(part.Name)
	}
	return b.String()
}

// fieldsCompatible reports whether b's fields are prefix-compatible with a.
// Items join a group when their shared leading field names match — extra
// trailing fields on either side are fine.
func fieldsCompatible(a, b []string) bool {
	n := min(len(a), len(b))
	for i := range n {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// findColumnGroup returns the exclusive end index of a column group
// starting at idx. Returns idx+1 (no group) if fewer than 2 consecutive
// one-line struct lits share the same signature with no intervening
// blank lines or comments.
func (p *printer) findColumnGroup(stmts []ast.Stmt, idx int) int {
	first := p.oneLineStructLit(stmts[idx])
	if first == nil {
		return idx + 1
	}
	typeName := columnTypeName(first)
	fields := columnFieldNames(first)
	end := idx + 1
	for end < len(stmts) {
		if p.hasBlankLineBetween(stmts[end-1].Span().End, stmts[end].Span().Start) {
			break
		}
		sl := p.oneLineStructLit(stmts[end])
		if sl == nil || columnTypeName(sl) != typeName || !fieldsCompatible(fields, columnFieldNames(sl)) {
			break
		}
		end++
	}
	return end
}

// emitColumnGroup formats a column group of one-line struct lits with
// aligned field values.
func (p *printer) emitColumnGroup(stmts []ast.Stmt) {
	lits := make([]*ast.StructLit, len(stmts))
	for i, s := range stmts {
		lits[i] = p.oneLineStructLit(s)
	}
	maxWidths := p.computeColumnWidths(lits)

	for _, s := range stmts {
		sl := p.oneLineStructLit(s)
		p.emitCommentsBefore(s.Span().Start)
		p.writeIndent()
		if sl.Type != nil {
			p.typeExpr(sl.Type)
			p.write(" ")
		}
		p.emitAlignedFields(sl.Fields, maxWidths)
		p.newline()
	}
}

// computeColumnWidths measures the max value width per column. Only
// items where the column is NOT the last field contribute — the last
// field is never padded, so its width shouldn't inflate padding for
// items that have more fields after it.
func (p *printer) computeColumnWidths(lits []*ast.StructLit) []int {
	maxCols := 0
	for _, sl := range lits {
		if len(sl.Fields) > maxCols {
			maxCols = len(sl.Fields)
		}
	}
	maxWidths := make([]int, maxCols)
	for _, sl := range lits {
		for col, fi := range sl.Fields {
			if col == len(sl.Fields)-1 {
				continue // last field — never padded, skip
			}
			w := p.measureExpr(fi.Value)
			if w > maxWidths[col] {
				maxWidths[col] = w
			}
		}
	}
	return maxWidths
}

// emitAlignedFields writes a one-line struct body `{ f = v, f = v }` with
// column-aligned values. Non-last fields are padded; the last field for
// each item is written tight.
func (p *printer) emitAlignedFields(fields []*ast.FieldInit, maxWidths []int) {
	nFields := len(fields)
	p.write("{ ")
	for col, fi := range fields {
		if col > 0 {
			p.write(" ")
		}
		p.write(fi.Name.Name + " = ")
		before := p.buf.Len()
		p.expr(fi.Value)
		w := p.buf.Len() - before
		if col < nFields-1 {
			p.write(",")
			if col < len(maxWidths) {
				if pad := maxWidths[col] - w; pad > 0 {
					p.write(strings.Repeat(" ", pad))
				}
			}
		}
	}
	p.write(" }")
}

// measureExpr formats an expression to a scratch buffer and returns its width.
func (p *printer) measureExpr(e ast.Expr) int {
	var scratch bytes.Buffer
	tmp := &printer{src: p.src, buf: &scratch, indent: p.indent}
	tmp.expr(e)
	return scratch.Len()
}

// oneLineStructLitExpr is like oneLineStructLit but works on an Expr
// directly (for list items, which are expressions not statements).
func (p *printer) oneLineStructLitExpr(e ast.Expr) *ast.StructLit {
	sl, ok := e.(*ast.StructLit)
	if !ok || len(sl.Fields) == 0 || len(sl.Body) > 0 {
		return nil
	}
	if p.spanContainsNewline(sl.SrcSpan) {
		return nil
	}
	return sl
}

// listItemsAligned emits list items, detecting column groups of
// consecutive one-line struct-lit elements and aligning them.
func (p *printer) listItemsAligned(items []ast.Expr) {
	var prevEnd uint32
	i := 0
	for i < len(items) {
		if i > 0 && p.hasBlankLineBetween(prevEnd, items[i].Span().Start) {
			p.newline()
		}
		// Try to form a column group of consecutive one-line struct lits.
		groupEnd := p.findListColumnGroup(items, i)
		if groupEnd > i+1 {
			p.emitListColumnGroup(items[i:groupEnd])
			prevEnd = items[groupEnd-1].Span().End
			i = groupEnd
			continue
		}
		p.emitCommentsBefore(items[i].Span().Start)
		p.writeIndent()
		p.expr(items[i])
		p.write(",")
		p.newline()
		prevEnd = items[i].Span().End
		i++
	}
}

func (p *printer) findListColumnGroup(items []ast.Expr, idx int) int {
	first := p.oneLineStructLitExpr(items[idx])
	if first == nil {
		return idx + 1
	}
	typeName := columnTypeName(first)
	fields := columnFieldNames(first)
	end := idx + 1
	for end < len(items) {
		if p.hasBlankLineBetween(items[end-1].Span().End, items[end].Span().Start) {
			break
		}
		sl := p.oneLineStructLitExpr(items[end])
		if sl == nil || columnTypeName(sl) != typeName || !fieldsCompatible(fields, columnFieldNames(sl)) {
			break
		}
		end++
	}
	return end
}

func (p *printer) emitListColumnGroup(items []ast.Expr) {
	lits := make([]*ast.StructLit, len(items))
	for i, item := range items {
		lits[i] = p.oneLineStructLitExpr(item)
	}
	maxWidths := p.computeColumnWidths(lits)

	var prevEnd uint32
	for i, item := range items {
		if i > 0 && p.hasBlankLineBetween(prevEnd, item.Span().Start) {
			p.newline()
		}
		sl := p.oneLineStructLitExpr(item)
		p.emitCommentsBefore(item.Span().Start)
		p.writeIndent()
		if sl.Type != nil {
			p.typeExpr(sl.Type)
			p.write(" ")
		}
		p.emitAlignedFields(sl.Fields, maxWidths)
		p.write(",")
		p.newline()
		prevEnd = item.Span().End
	}
}
