package engine

import (
	"context"
	"fmt"
)

func Apply(ctx context.Context, cfg string) error {
	fmt.Printf("doit apply, cfg=%s, ctx=%v\n", cfg, ctx)
	return nil
}
