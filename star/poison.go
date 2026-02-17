// SPDX-License-Identifier: GPL-3.0-only

package star

import (
	"fmt"

	"go.starlark.net/starlark"
)

// poisonValue is returned by top-level declaration builtins (deploy, secrets,
// target.*) to prevent their results from being used as values. If someone
// writes `x = secrets(...)`, x will carry a poisonValue that renders as an
// obvious error marker in any output.
type poisonValue struct {
	funcName string
}

func (p poisonValue) String() string {
	return fmt.Sprintf("<%s() is a declaration, not a value>", p.funcName)
}
func (p poisonValue) Type() string         { return "declaration" }
func (p poisonValue) Freeze()              {}
func (p poisonValue) Truth() starlark.Bool { return starlark.False }
func (p poisonValue) Hash() (uint32, error) {
	return 0, fmt.Errorf("%s() is a top-level declaration and cannot be used as a value", p.funcName)
}

// checkPoison returns an error if v is a poisonValue or contains one nested
// inside a list, tuple, or dict.
func checkPoison(v starlark.Value) error {
	switch v := v.(type) {
	case poisonValue:
		return fmt.Errorf("%s() is a top-level declaration and cannot be used as a value", v.funcName)
	case *starlark.List:
		for i := 0; i < v.Len(); i++ {
			if err := checkPoison(v.Index(i)); err != nil {
				return err
			}
		}
	case starlark.Tuple:
		for _, elem := range v {
			if err := checkPoison(elem); err != nil {
				return err
			}
		}
	case *starlark.Dict:
		for _, item := range v.Items() {
			if err := checkPoison(item[1]); err != nil {
				return err
			}
		}
	}
	return nil
}
