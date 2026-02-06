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
	// drop and reconnect, producing warning/error logs that corrupt the TUI.
	// Both settings are needed: SetOutput redirects the logger, but errors
	// and warnings still go to stderr unless LogToStderr is disabled.
	klog.SetOutput(io.Discard)
	klog.LogToStderr(false)
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
