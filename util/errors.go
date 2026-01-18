package util

import "fmt"

func BUG(format string, a ...any) error {
	return fmt.Errorf("BUG: "+format, a...)
}
