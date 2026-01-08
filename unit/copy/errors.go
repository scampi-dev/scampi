package copy

import (
	"fmt"

	"godoit.dev/doit/diagnostic"
)

type (
	InvalidOctal struct {
		Value    string
		Regex    string
		Examples []string
		Err      error
	}
)

func (e InvalidOctal) Error() string { return fmt.Sprintf("invalid octal '%s' - %s", e.Value, e.Err) }
func (e InvalidOctal) Template() diagnostic.Template {
	return diagnostic.Template{
		Name: "copy.invalidOctal",
		Text: "invalid octal format '{{.Value}}'",
		Hint: `valid regex '{{.Regex}}', i.e. {{join ", " .Examples}}`,
	}
}
