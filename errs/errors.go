package errs

import "fmt"

func BUG(format string, args ...any) error {
	return fmt.Errorf("BUG: "+format, args...)
}

func Errorf(format string, args ...any) error {
	return fmt.Errorf(format, args...)
}

func WrapErrf(err error, format string, args ...any) error {
	concat := []any{err}
	concat = append(concat, args...)
	return Errorf("%w: "+format, concat...)
}
