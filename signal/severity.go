//go:generate stringer -type=Severity
package signal

type Severity uint8

const (
	Debug Severity = iota
	Info
	Notice
	Warning
	Error
	Fatal
)
