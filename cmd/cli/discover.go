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
	return discoverCmd(
		func() (contract.GameDiscovery, error) { return c.GameDiscovery() },
		func() (contract.MapsLocator, error) { return c.MapsLocator() },
	)
}

// discoverCmd checks that the running game can be found, without opening
// the window. The session is never printed, only its length.
func discoverCmd(resolve func() (contract.GameDiscovery, error), resolveMaps func() (contract.MapsLocator, error)) *cobra.Command {
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

			locator, err := resolveMaps()
			if err != nil {
				return fmt.Errorf("discover: %w", err)
			}
			out := cmd.OutOrStdout()
			// Maps folders are found from known places even without the game.
			folders := locator.MapsFolders(ctx)
			_, _ = fmt.Fprintln(out, "Maps folders:")
			if len(folders) == 0 {
				_, _ = fmt.Fprintln(out, "  none found; choose it in the window")
			}
			for _, f := range folders {
				_, _ = fmt.Fprintf(out, "  %s  (%s)\n", f.Path, f.Source)
			}
			if len(folders) > 0 {
				gamePath, _ := locator.GamePath(ctx, folders[0].Path)
				_, _ = fmt.Fprintf(out, "  The game sees the first as: %s\n", gamePath)
			}

			conn, err := discovery.Discover(ctx)
			if err != nil {
				return fmt.Errorf("discover: %w", err)
			}
			_, err = fmt.Fprintf(out,
				"Found Warcraft III (process %d)\nVersion:      %s\nMenu port:    %d\nMenu session: found (%d digits, not shown)\n",
				conn.PID, conn.Version, conn.Port, len(conn.Session))
			return err
		},
	}
}
