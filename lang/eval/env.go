// SPDX-License-Identifier: GPL-3.0-only

package eval

// envScope is a runtime variable scope. Separate from check.Scope
// because it holds Values, not Types.
type envScope struct {
	parent *envScope
	vars   map[string]Value
}

func newEnv(parent *envScope) *envScope {
	return &envScope{parent: parent, vars: make(map[string]Value)}
}

func (e *envScope) set(name string, v Value) {
	e.vars[name] = v
}

func (e *envScope) get(name string) (Value, bool) {
	if v, ok := e.vars[name]; ok {
		return v, true
	}
	if e.parent != nil {
		return e.parent.get(name)
	}
	return nil, false
}
