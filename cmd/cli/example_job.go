package cli

import (
	"fmt"
	"log/slog"

	"github.com/spf13/cobra"

	"github.com/mihaiflorentin/wc3-team-follow/container"
	"github.com/mihaiflorentin/wc3-team-follow/port/contract"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

// jobDeps is what the example-job command resolves from the container.
type jobDeps struct {
	Logger  *slog.Logger
	Example contract.ExampleService
}

func newExampleJobCmd(c *container.ServiceContainer) *cobra.Command {
	return exampleJobCmd(func() (jobDeps, error) { return resolveJobDeps(c) })
}

// resolveJobDeps resolves the job's dependencies from the container. A
// backend that cannot be reached fails here, before the job runs.
func resolveJobDeps(c *container.ServiceContainer) (jobDeps, error) {
	deps := jobDeps{Logger: c.Logger()}
	var err error
	if deps.Example, err = c.ExampleService(); err != nil {
		return deps, fmt.Errorf("resolve example service: %w", err)
	}
	return deps, nil
}

// exampleJobCmd builds the command around resolve, so tests run it with test
// doubles instead of a container.
func exampleJobCmd(resolve func() (jobDeps, error)) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "example-job",
		Short: "Runs the example job once and exits",
		Long:  "Runs one example job (ExampleService.RunJob) and exits: 0 on success, non-zero on failure. Schedulers such as Kubernetes CronJobs run it.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			deps, err := resolve()
			if err != nil {
				return fmt.Errorf("example-job: %w", err)
			}
			name, _ := cmd.Flags().GetString("name")

			ctx, stop := shutdownContext(cmd.Context())
			defer stop()
			if err := deps.Example.RunJob(ctx, dto.ExampleJob{Name: name}); err != nil {
				return fmt.Errorf("example-job: %w", err)
			}
			deps.Logger.InfoContext(ctx, "example-job.done", slog.String("name", name))
			return nil
		},
	}
	cmd.Flags().String("name", "example", "Name of the example job to run")
	return cmd
}
