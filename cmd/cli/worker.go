package cli

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/spf13/cobra"

	"github.com/mihaiflorentin/wc3-team-follow/container"
	"github.com/mihaiflorentin/wc3-team-follow/port/contract"
)

// workerDeps is what the worker command resolves from the container.
type workerDeps struct {
	Logger  *slog.Logger
	Example contract.ExampleService
}

func newWorkerCmd(c *container.ServiceContainer) *cobra.Command {
	return workerCmd(func() (workerDeps, error) { return resolveWorkerDeps(c) })
}

// resolveWorkerDeps resolves the worker's dependencies from the container. A
// backend that cannot be reached fails here, before the loop starts.
func resolveWorkerDeps(c *container.ServiceContainer) (workerDeps, error) {
	deps := workerDeps{Logger: c.Logger()}
	var err error
	if deps.Example, err = c.ExampleService(); err != nil {
		return deps, fmt.Errorf("resolve example service: %w", err)
	}
	return deps, nil
}

// workerCmd builds the command around resolve, so tests run it with test
// doubles instead of a container.
func workerCmd(resolve func() (workerDeps, error)) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "worker",
		Short: "Runs the long-lived example worker until SIGTERM",
		Long:  "Runs ExampleService.Ping every --interval as a placeholder loop. Stops cleanly on SIGINT or SIGTERM.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			deps, err := resolve()
			if err != nil {
				return fmt.Errorf("worker: %w", err)
			}
			ctx, stop := shutdownContext(cmd.Context())
			defer stop()
			interval, _ := cmd.Flags().GetDuration("interval")
			if interval <= 0 {
				return fmt.Errorf("worker: --interval must be positive, got %s", interval)
			}
			runPingLoop(ctx, deps, interval)
			deps.Logger.InfoContext(context.WithoutCancel(ctx), "worker.stopped")
			return nil
		},
	}
	cmd.Flags().Duration("interval", 30*time.Second, "Time between two runs of the loop")
	return cmd
}

// runPingLoop calls ExampleService.Ping every interval until ctx ends. A
// failed run is logged and the loop carries on. Replace it with your own work.
func runPingLoop(ctx context.Context, deps workerDeps, interval time.Duration) {
	deps.Logger.InfoContext(ctx, "worker.polling", slog.Duration("interval", interval))
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			status, err := deps.Example.Ping(ctx)
			if err != nil {
				deps.Logger.ErrorContext(ctx, "worker.ping_failed", slog.Any("error", err))
				continue
			}
			deps.Logger.InfoContext(ctx, "worker.ping", slog.Int("examples", status.Examples))
		}
	}
}
