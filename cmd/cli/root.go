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
// here, at construction, and passes resolved dependencies on. Running the
// program without a command opens the window.
func NewRootCmd(c *container.ServiceContainer) *cobra.Command {
	root := newWindowCmd(c)
	root.Use = "wc3-team-follow"
	root.Short = "Follow your Warcraft III friends into their lobbies"
	root.Version = fmt.Sprintf("%s (commit: %s, built: %s)", version, commit, date)
	root.SilenceUsage = true
	root.SilenceErrors = true
	root.AddCommand(newDiscoverCmd(c))
	return root
}

// BuildInfo returns the version, commit and build date injected at build
// time, for the start-up log line.
func BuildInfo() (string, string, string) { return version, commit, date }

// Execute runs the command tree built from c.
func Execute(c *container.ServiceContainer) error {
	return NewRootCmd(c).Execute()
}
