// SPDX-License-Identifier: GPL-3.0-only

package engine

import "context"

// Code is the stable identifier on every emission. Sinks classify
// by exact match.
type Code string

const (
	CodeSnapshotReceived Code = "snapshot.received"
	CodeSnapshotRejected Code = "snapshot.rejected"
	CodeApplyStart       Code = "apply.start"
	CodeApplySuccess     Code = "apply.success"
	CodeApplyFailed      Code = "apply.failed"
	CodeApplyHalted      Code = "apply.halted"
	CodeApplyRenamed     Code = "apply.renamed"
	CodeTickComplete     Code = "tick.complete"
	CodeDestroyStart     Code = "destroy.start"
	CodeDestroySuccess   Code = "destroy.success"
	CodeDestroyFailed    Code = "destroy.failed"

	CodeMeshUp          Code = "mesh.up"
	CodeMeshDown        Code = "mesh.down"
	CodeMeshUnavailable Code = "mesh.unavailable"
	CodeMeshPeerJoined  Code = "mesh.peer.joined"
	CodeMeshPeerLeft    Code = "mesh.peer.left"
	CodeMeshPeerUpdated Code = "mesh.peer.updated"

	CodeLogDebug Code = "log.debug"
	CodeLogInfo  Code = "log.info"
	CodeLogWarn  Code = "log.warn"
	CodeLogError Code = "log.error"
)

// IsLifecycle reports whether c is a structural lifecycle event
// rather than a convenience log emission.
func (c Code) IsLifecycle() bool {
	switch c {
	case CodeLogDebug, CodeLogInfo, CodeLogWarn, CodeLogError:
		return false
	}
	return true
}

// Emitter is the sink contract. Err is sticky: once a sink fails it
// stays failed, so the reconcile loop can abort the pass on first
// failure instead of acting without recording.
type Emitter interface {
	Emit(ctx context.Context, code Code, ref *Ref, args ...any)
	Err() error
}

type FanoutEmitter []Emitter

func (f FanoutEmitter) Emit(ctx context.Context, code Code, ref *Ref, args ...any) {
	for _, e := range f {
		e.Emit(ctx, code, ref, args...)
	}
}

func (f FanoutEmitter) Err() error {
	for _, e := range f {
		if err := e.Err(); err != nil {
			return err
		}
	}
	return nil
}

type Log struct {
	e Emitter
}

func NewLog(e Emitter) Log { return Log{e: e} }

func (l Log) Emit(ctx context.Context, code Code, ref *Ref, args ...any) {
	l.e.Emit(ctx, code, ref, args...)
}

func (l Log) Err() error { return l.e.Err() }

func (l Log) Debug(ctx context.Context, msg string, args ...any) {
	l.emitLog(ctx, CodeLogDebug, msg, args)
}

func (l Log) Info(ctx context.Context, msg string, args ...any) {
	l.emitLog(ctx, CodeLogInfo, msg, args)
}

func (l Log) Warn(ctx context.Context, msg string, args ...any) {
	l.emitLog(ctx, CodeLogWarn, msg, args)
}

func (l Log) Error(ctx context.Context, msg string, args ...any) {
	l.emitLog(ctx, CodeLogError, msg, args)
}

func (l Log) emitLog(ctx context.Context, code Code, msg string, args []any) {
	full := make([]any, 0, len(args)+2)
	full = append(full, "msg", msg)
	full = append(full, args...)
	l.Emit(ctx, code, nil, full...)
}

type discardEmitter struct{}

func (discardEmitter) Emit(context.Context, Code, *Ref, ...any) {}
func (discardEmitter) Err() error                               { return nil }

var Discard Emitter = discardEmitter{}
