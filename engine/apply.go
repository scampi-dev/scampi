package engine

import (
	"context"
	"fmt"

	"cuelang.org/go/cue/errors"
)

func Apply(ctx context.Context, cfgPath string) error {
	cfg, err := loadAndValidate(cfgPath)
	if err != nil {
		errs := errors.Errors(err)
		fmt.Printf("CUE error summary:\n%v\n", err)
		fmt.Printf("CUE error details:\n%v\n", errors.Details(err, nil))
		fmt.Printf("CUE: %d error(s)\n", len(errs))
		return err
	}

	fmt.Printf("decoded config: %v\n", cfg)

	return nil
}
