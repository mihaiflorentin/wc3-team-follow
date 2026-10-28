package cli

import (
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/spf13/cobra"

	"github.com/mihaiflorentin/wc3-team-follow/container"
	"github.com/mihaiflorentin/wc3-team-follow/domain/follow"
	"github.com/mihaiflorentin/wc3-team-follow/port/contract"
)

func newTypeCmd(c *container.ServiceContainer) *cobra.Command {
	return typeCmd(
		func() (contract.GameDiscovery, error) { return c.GameDiscovery() },
		func() (contract.GameKeyboard, error) { return c.GameKeyboard() },
	)
}

// typeCmd types one line in the match chat with keystrokes, as Game mode
// would: a diagnostic for checking keystroke input by hand (docs/adr/0007).
// It never brings the game forward; the player does that during --delay,
// and typing stops the moment the game isn't the window in front. It
// prints the line, the process id and the version, never the session.
func typeCmd(resolve func() (contract.GameDiscovery, error), resolveKeyboard func() (contract.GameKeyboard, error)) *cobra.Command {
	var delay time.Duration
	var ask bool
	cmd := &cobra.Command{
		Use:   "type [flags] -- <text>",
		Short: "Types one line in the match chat with keystrokes",
		Long: "Types one line in Warcraft III's match chat with keystrokes: Enter, the text, Enter.\n" +
			"Bring the game to the front during --delay; typing stops if it isn't in front.\n" +
			"Put -- before text that starts with a dash: wc3-team-follow type -- -ar",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			keyboard, err := resolveKeyboard()
			if err != nil {
				return fmt.Errorf("type: %w", err)
			}
			discovery, err := resolve()
			if err != nil {
				return fmt.Errorf("type: %w", err)
			}
			line, err := follow.ChatLine(args[0])
			if err != nil {
				return fmt.Errorf("type: %w", err)
			}
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
			defer stop()
			if st := keyboard.Status(ctx); !st.Supported || !st.Permitted {
				if !st.Supported || !ask {
					return fmt.Errorf("type: %s", st.Reason)
				}
				if err := keyboard.RequestPermission(ctx); err != nil {
					return fmt.Errorf("type: %w", err)
				}
			}
			conn, err := discovery.Discover(ctx)
			if err != nil {
				return fmt.Errorf("type: %w", err)
			}

			out := cmd.OutOrStdout()
			_, _ = fmt.Fprintf(out, "%s typing %q in Warcraft III (process %d, %s) in %s: bring the game to the front\n", stamp(), line, conn.PID, conn.Version, delay)
			select {
			case <-ctx.Done():
				return fmt.Errorf("type: %w", ctx.Err())
			case <-time.After(delay):
			}
			if err := keyboard.TypeLine(ctx, conn.PID, line); err != nil {
				return fmt.Errorf("type: %w", err)
			}
			_, _ = fmt.Fprintf(out, "%s typed %q; the game doesn't confirm that it showed\n", stamp(), line)
			return nil
		},
	}
	cmd.Flags().DurationVar(&delay, "delay", 3*time.Second, "how long to wait before typing, to bring the game to the front")
	cmd.Flags().BoolVar(&ask, "ask-permission", false, "ask the system for the keystroke permission if it isn't granted (macOS)")
	return cmd
}
