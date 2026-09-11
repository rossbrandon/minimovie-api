package main

import (
	"context"
	"errors"
	"fmt"
	"os"
)

func main() {
	ctx, stop := signalContext(context.Background())
	err := newRootCmd().ExecuteContext(ctx)
	stop()

	if err != nil && !errors.Is(err, errInterrupted) {
		fmt.Fprintf(os.Stderr, "seed: %v\n", err)
	}
	os.Exit(exitCode(err))
}
