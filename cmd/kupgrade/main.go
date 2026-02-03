package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/sabirmohamed/kupgrade/internal/cli"
	"k8s.io/klog/v2"
)

func init() {
	// Silence client-go's klog output. During upgrades, informer watches
	// drop and reconnect, producing warning logs that corrupt the TUI.
	klog.SetOutput(io.Discard)
}

func main() {
	if err := cli.Execute(); err != nil {
		var exitErr *cli.ExitCodeError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.Code)
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
