package render

import "godoit.dev/doit/signal"

type ColorMode uint8

const (
	ColorAuto ColorMode = iota
	ColorAlways
	ColorNever
)

type Color uint8

const (
	DebugHighlight Color = iota
	DebugNormal
	DebugDimmed

	InfoHighlight
	InfoNormal
	InfoDimmed

	NoticeHighlight
	NoticeNormal
	NoticeDimmed

	ImportantHighlight
	ImportantNormal
	ImportantDimmed

	WarningHighlight
	WarningNormal
	WarningDimmed

	ErrorHighlight
	ErrorNormal
	ErrorDimmed

	FatalHighlight
	FatalNormal
	FatalDimmed
)

type SeverityColors struct {
	Highlight Color
	Normal    Color
	Dimmed    Color
}

func ColorsForSeverity(s signal.Severity) SeverityColors {
	switch s {
	case signal.Debug:
		return SeverityColors{
			Highlight: DebugHighlight,
			Normal:    DebugNormal,
			Dimmed:    DebugDimmed,
		}

	case signal.Info:
		return SeverityColors{
			Highlight: InfoHighlight,
			Normal:    InfoNormal,
			Dimmed:    InfoDimmed,
		}

	case signal.Notice:
		return SeverityColors{
			Highlight: NoticeHighlight,
			Normal:    NoticeNormal,
			Dimmed:    NoticeDimmed,
		}

	case signal.Important:
		return SeverityColors{
			Highlight: ImportantHighlight,
			Normal:    ImportantNormal,
			Dimmed:    ImportantDimmed,
		}

	case signal.Warning:
		return SeverityColors{
			Highlight: WarningHighlight,
			Normal:    WarningNormal,
			Dimmed:    WarningDimmed,
		}

	case signal.Error:
		return SeverityColors{
			Highlight: ErrorHighlight,
			Normal:    ErrorNormal,
			Dimmed:    ErrorDimmed,
		}

	case signal.Fatal:
		return SeverityColors{
			Highlight: FatalHighlight,
			Normal:    FatalNormal,
			Dimmed:    FatalDimmed,
		}

	default:
		return SeverityColors{
			Highlight: InfoHighlight,
			Normal:    InfoNormal,
			Dimmed:    InfoDimmed,
		}
	}
}
