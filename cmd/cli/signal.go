package cli

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

// shutdownContext returns a context cancelled on SIGINT or SIGTERM, the
// signal Kubernetes sends before it stops a pod. Only portable signals are
// used: os.Interrupt and SIGTERM exist on every GOOS.
func shutdownContext(parent context.Context) (context.Context, context.CancelFunc) {
	return signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
}
