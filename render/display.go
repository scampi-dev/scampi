package render

import (
	"godoit.dev/doit/diagnostic/event"
)

type Template struct {
	Name string
	Text string
	Hint string
	Help string

	Data any
}

type RunSummary struct {
	ChangedCount int
	FailedCount  int
	TotalCount   int
}

type Displayer interface {
	Emit(e event.Event)
}

func s(n int) string {
	if n == 1 {
		return ""
	}

	return "s"
}
