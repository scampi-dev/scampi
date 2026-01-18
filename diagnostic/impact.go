package diagnostic

type Impact uint8

const (
	ImpactAbort Impact = 1 << iota
	ImpactNone  Impact = 0
)

type ImpactProvider interface {
	Impact() Impact
}
