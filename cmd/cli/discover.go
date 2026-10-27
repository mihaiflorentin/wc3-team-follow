package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/spf13/cobra"

	"github.com/mihaiflorentin/wc3-team-follow/container"
	"github.com/mihaiflorentin/wc3-team-follow/port/contract"
)

func newDiscoverCmd(c *container.ServiceContainer) *cobra.Command {
	return discoverCmd(func() (contract.GameDiscovery, error) { return c.GameDiscovery() })
}

// discoverCmd checks that the running game can be found, without opening
// the window. The session is never printed, only its length.
func discoverCmd(resolve func() (contract.GameDiscovery, error)) *cobra.Command {
	return &cobra.Command{
		Use:   "discover",
		Short: "Checks that Team Follow can find the running game",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			discovery, err := resolve()
			if err != nil {
				return fmt.Errorf("discover: %w", err)
			}
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
			defer stop()
			ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()

			conn, err := discovery.Discover(ctx)
			if err != nil {
				return fmt.Errorf("discover: %w", err)
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(),
				"Found Warcraft III (process %d)\nVersion:      %s\nMenu port:    %d\nMenu session: found (%d digits, not shown)\n",
				conn.PID, conn.Version, conn.Port, len(conn.Session))
			return err
		},
	}
}
