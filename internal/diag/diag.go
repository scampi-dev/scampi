// SPDX-License-Identifier: GPL-3.0-only

// Package diag is scampi's structured-log interface.
package diag

import "context"

// Logger is a structured leveled logger.
type Logger interface {
	Debug(ctx context.Context, msg string, args ...any)
	Info(ctx context.Context, msg string, args ...any)
	Warn(ctx context.Context, msg string, args ...any)
	Error(ctx context.Context, msg string, args ...any)
}

// Discard drops every emission.
type Discard struct{}

func (Discard) Debug(context.Context, string, ...any) {}
func (Discard) Info(context.Context, string, ...any)  {}
func (Discard) Warn(context.Context, string, ...any)  {}
func (Discard) Error(context.Context, string, ...any) {}
