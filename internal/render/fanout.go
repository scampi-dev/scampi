// SPDX-License-Identifier: GPL-3.0-only

package render

import (
	"context"

	"scampi.dev/scampi/internal/engine"
)

type FanoutEmitter []engine.Emitter

func (f FanoutEmitter) Emit(ctx context.Context, code engine.Code, ref *engine.Ref, args ...any) {
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
