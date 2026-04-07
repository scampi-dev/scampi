// SPDX-License-Identifier: GPL-3.0-only

package check

import "scampi.dev/scampi/lang/token"

// Scope is a lexical scope that maps names to their types. Scopes
// chain to a parent for lookup; a nil parent is the universe scope.
type Scope struct {
	parent  *Scope
	symbols map[string]*Symbol
	kind    ScopeKind
}

// ScopeKind identifies the purpose of a scope (for mutability rules).
type ScopeKind uint8

const (
	ScopeFile  ScopeKind = iota // top-level file scope
	ScopeFunc                   // inside a func body — mutation allowed
	ScopeDecl                   // inside a step body — frozen
	ScopeBlock                  // for/if/else block — inherits parent kind
)

// Symbol is a named binding in a scope.
type Symbol struct {
	Name string
	Type Type
	Kind SymbolKind
	Span token.Span // definition site
}

// SymbolKind identifies what a name refers to.
type SymbolKind uint8

const (
	SymLet    SymbolKind = iota // let binding
	SymParam                    // function/step parameter
	SymFunc                     // function declaration
	SymDecl                     // step declaration
	SymType                     // struct type
	SymEnum                     // enum type
	SymImport                   // imported module namespace
)

// NewScope creates a child scope of parent.
func NewScope(parent *Scope, kind ScopeKind) *Scope {
	return &Scope{
		parent:  parent,
		symbols: make(map[string]*Symbol),
		kind:    kind,
	}
}

// Define adds a symbol to this scope. Returns false if the name is
// already defined in this scope (not parent scopes — shadowing is OK).
func (s *Scope) Define(sym *Symbol) bool {
	if _, exists := s.symbols[sym.Name]; exists {
		return false
	}
	s.symbols[sym.Name] = sym
	return true
}

// Lookup searches this scope and all parent scopes for a name.
// Returns nil if not found.
func (s *Scope) Lookup(name string) *Symbol {
	if sym, ok := s.symbols[name]; ok {
		return sym
	}
	if s.parent != nil {
		return s.parent.Lookup(name)
	}
	return nil
}

// AllowsMutation reports whether this scope (or an enclosing one)
// permits collection mutation. Only func-body scopes allow it.
func (s *Scope) AllowsMutation() bool {
	switch s.kind {
	case ScopeFunc:
		return true
	case ScopeBlock:
		if s.parent != nil {
			return s.parent.AllowsMutation()
		}
	}
	return false
}
