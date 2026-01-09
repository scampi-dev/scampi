package copy

import (
	"fmt"

	"godoit.dev/doit/diagnostic/event"
	"godoit.dev/doit/signal"
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
func (e InvalidOctal) EventTemplate() event.Template {
	return event.Template{
		ID:   "builtin.InvalidOctal",
		Text: "invalid octal format '{{.Value}}'",
		Hint: `valid regex '{{.Regex}}', i.e. {{join ", " .Examples}}`,
		Help: `help text here`,
		Data: e,
	}
}
func (e InvalidOctal) Severity() signal.Severity { return signal.Error }
