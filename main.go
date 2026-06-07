package main

import (
	"context"
	"fmt"
	"os"

	"github.com/jfrog/auto-fix/action"
)

func main() {
	in, err := action.ReadInputs()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading inputs: %v\n", err)
		os.Exit(1)
	}

	if err = action.Run(context.Background(), in); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
