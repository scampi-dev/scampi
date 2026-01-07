package signal

type Severity uint8

const (
	Debug Severity = iota
	Info
	Notice
	Important
	Warning
	Error
	Fatal
)
