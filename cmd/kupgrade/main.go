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
	// Fully silence client-go's klog output. During control plane upgrades,
	// the API server restarts and informer watches drop/reconnect, producing
	// reflector error/warning logs that corrupt the Bubble Tea TUI display.
	//
	// Three settings are needed because klog v2 has multiple output paths:
	// 1. SetOutput(Discard) — redirects the standard log writer
	// 2. LogToStderr(false) — prevents fallback to os.Stderr
	// 3. SetLogFilter — drops all messages before they reach any writer
	klog.SetOutput(io.Discard)
	klog.LogToStderr(false)
	klog.SetLogFilter(&discardFilter{})
}

// discardFilter implements klog.LogFilter to drop all log messages.
type discardFilter struct{}

func (d *discardFilter) Filter(args []interface{}) []interface{} { return nil }
func (d *discardFilter) FilterF(format string, args []interface{}) (string, []interface{}) {
	return "", nil
}
func (d *discardFilter) FilterS(msg string, keysAndValues []interface{}) (string, []interface{}) {
	return "", nil
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
