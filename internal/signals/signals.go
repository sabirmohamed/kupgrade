package signals

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

var shutdownSignals = []os.Signal{os.Interrupt, syscall.SIGTERM}

// SetupSignalHandler registers for SIGTERM and SIGINT.
// Returns a context that is canceled on first signal.
// A second signal causes immediate exit.
func SetupSignalHandler() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	c := make(chan os.Signal, 2)
	signal.Notify(c, shutdownSignals...)
	go func() {
		<-c
		cancel() // First signal: graceful shutdown
		<-c
		os.Exit(1) // Second signal: force exit
	}()
	return ctx
}
