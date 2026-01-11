package diagnostic

type Impact uint8

const (
	ImpactSkipUnit Impact = 1 << iota
	ImpactSkipAction
	ImpactAbort
	ImpactNone Impact = 0
)

type ImpactProvider interface {
	Impact() Impact
}
