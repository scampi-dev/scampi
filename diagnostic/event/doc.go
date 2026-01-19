// Package event defines the structured diagnostic event model.
//
// Events are immutable records carrying timing, scope, severity, and typed
// details. They form a tagged union consumed by renderers and other observers.
// This package contains no execution or rendering logic.
package event
