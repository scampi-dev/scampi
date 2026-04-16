// SPDX-License-Identifier: GPL-3.0-only

package errs

import (
	"errors"
	"fmt"
)

// Code is a stable diagnostic error identifier (e.g. "parse.MissingModuleDecl").
// Surfaced by the LSP and documented on scampi.dev/errors/.
type Code string

func BUG(format string, args ...any) error {
	return fmt.Errorf("BUG: "+format, args...)
}

func New(text string) error {
	return errors.New(text)
}

func Errorf(format string, args ...any) error {
	return fmt.Errorf(format, args...)
}

func WrapErrf(err error, format string, args ...any) error {
	concat := []any{err}
	concat = append(concat, args...)
	return Errorf("%w: "+format, concat...)
}

func UnwrapAll(err error) error {
	for {
		next := errors.Unwrap(err)
		if next == nil {
			return err
		}
		err = next
	}
}
