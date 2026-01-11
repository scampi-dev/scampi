package diagnostic

type Effect uint8

const (
	EffectContinue Effect = iota
	EffectAbort
	EffectSkipUnit
	EffectSkipAction
)

type EffectProvider interface {
	Effect() Effect
}
