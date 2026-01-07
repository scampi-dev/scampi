package diagnostic

type Verbosity uint8

const (
	Quiet Verbosity = iota // default
	Verbose
	VeryVerbose
	DebugVerbose
)
