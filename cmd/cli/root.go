// Package cli is the wiring layer: it builds the command tree from the
// service container and resolves each command's dependencies from it.
package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mihaiflorentin/wc3-team-follow/container"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// NewRootCmd builds the command tree. Every command receives the container
// here, at construction, and passes resolved dependencies on.
func NewRootCmd(c *container.ServiceContainer) *cobra.Command {
	root := &cobra.Command{
		Use:           "wc3-team-follow",
		Short:         "wc3-team-follow command line interface",
		Long:          "wc3-team-follow exposes operational commands for the service.",
		Version:       fmt.Sprintf("%s (commit: %s, built: %s)", version, commit, date),
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(newServerCmd(c))
	root.AddCommand(newWorkerCmd(c))
	root.AddCommand(newExampleJobCmd(c))
	return root
}

// Execute runs the command tree built from c.
func Execute(c *container.ServiceContainer) error {
	return NewRootCmd(c).Execute()
}
