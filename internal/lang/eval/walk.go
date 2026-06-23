// SPDX-License-Identifier: GPL-3.0-only

package eval

// WalkFunc is called for each value in the tree. Return false to
// skip children of the current value.
type WalkFunc func(v Value) bool

// Walk traverses a value tree depth-first. The callback is called
// before visiting children. If it returns false, children are skipped.
func Walk(v Value, fn WalkFunc) {
	if v == nil {
		return
	}
	if !fn(v) {
		return
	}
	switch v := v.(type) {
	case *StructVal:
		for _, fv := range v.Fields {
			Walk(fv, fn)
		}
	case *BlockResultVal:
		for _, fv := range v.Fields {
			Walk(fv, fn)
		}
		for _, bv := range v.Body {
			Walk(bv, fn)
		}
	case *BlockVal:
		for _, fv := range v.Fields {
			Walk(fv, fn)
		}
	case *ListVal:
		for _, item := range v.Items {
			Walk(item, fn)
		}
	case *MapVal:
		for i, k := range v.Keys {
			Walk(k, fn)
			Walk(v.Values[i], fn)
		}
	}
}

// WalkResult walks all values in a Result (bindings + expressions).
func WalkResult(r *Result, fn WalkFunc) {
	for _, v := range r.Bindings {
		Walk(v, fn)
	}
	for _, v := range r.Exprs {
		Walk(v, fn)
	}
}
