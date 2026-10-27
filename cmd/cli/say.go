package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"time"

	"github.com/spf13/cobra"

	"github.com/mihaiflorentin/wc3-team-follow/container"
	"github.com/mihaiflorentin/wc3-team-follow/domain/follow"
	"github.com/mihaiflorentin/wc3-team-follow/port/contract"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

func newSayCmd(c *container.ServiceContainer) *cobra.Command {
	return sayCmd(
		func() (contract.GameDiscovery, error) { return c.GameDiscovery() },
		func() (contract.GameDialer, error) { return c.GameDialer() },
	)
}

// sayCmd types one line in the game chat, as Auto-host would, and prints
// what the game answers: a diagnostic for checking the chat by hand
// (docs/adr/0006). It prints event names, chat lines and channel names
// only, never the session, account ids or whole payloads.
func sayCmd(resolve func() (contract.GameDiscovery, error), resolveDialer func() (contract.GameDialer, error)) *cobra.Command {
	var wait time.Duration
	cmd := &cobra.Command{
		Use:   "say <text>",
		Short: "Types one line in the game chat and shows what the game answers",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			line, err := follow.ChatLine(args[0])
			if err != nil {
				return fmt.Errorf("say: %w", err)
			}
			discovery, err := resolve()
			if err != nil {
				return fmt.Errorf("say: %w", err)
			}
			dialer, err := resolveDialer()
			if err != nil {
				return fmt.Errorf("say: %w", err)
			}
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
			defer stop()
			conn, err := discovery.Discover(ctx)
			if err != nil {
				return fmt.Errorf("say: %w", err)
			}
			session, err := dialer.Dial(ctx, conn)
			if err != nil {
				return fmt.Errorf("say: %w", err)
			}
			defer func() { _ = session.Close() }()

			out := cmd.OutOrStdout()
			if err := session.Send(ctx, follow.CmdSendGameChatMessage, map[string]any{"content": line}); err != nil {
				return fmt.Errorf("say: %w", err)
			}
			_, _ = fmt.Fprintf(out, "%s sent %q to Warcraft III (process %d, %s); listening for %s\n", stamp(), line, conn.PID, conn.Version, wait)
			ctx, cancel := context.WithTimeout(ctx, wait)
			defer cancel()
			for {
				ev, err := session.Receive(ctx)
				if err != nil {
					if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(ctx.Err(), context.Canceled) {
						return nil
					}
					return fmt.Errorf("say: %w", err)
				}
				printEvent(out, ev)
			}
		},
	}
	cmd.Flags().DurationVar(&wait, "wait", 10*time.Second, "how long to show the game's events after sending")
	return cmd
}

func stamp() string { return time.Now().Format("15:04:05.000") }

// printEvent writes one event without its payload, except the few fields
// that tell whether a chat line arrived or when a match finished loading.
func printEvent(out io.Writer, ev dto.GameEvent) {
	if ev.Malformed {
		return
	}
	detail := ""
	p := ev.Payload
	switch ev.MessageType {
	case "ChatMessage":
		m, _ := p["message"].(map[string]any)
		detail = fmt.Sprintf(" source=%v type=%v content=%q", m["source"], m["type"], m["content"])
	case "OnChannelJoin", "OnChannelLeave":
		for _, key := range []string{"gameChat", "channel"} {
			if c, ok := p[key].(map[string]any); ok {
				detail = fmt.Sprintf(" %s name=%v", key, firstOf(c, "name", "displayName"))
			}
		}
	case "SetGlueScreen":
		detail = fmt.Sprintf(" screen=%v", p["screen"])
	case "IsGameUIActive":
		detail = fmt.Sprintf(" isActive=%v", p["isActive"])
	case "LoadProgressUpdate":
		d, _ := p["details"].(map[string]any)
		detail = fmt.Sprintf(" fill=%v", d["fillPercentage"])
	}
	_, _ = fmt.Fprintf(out, "%s %s%s\n", stamp(), ev.MessageType, detail)
}

func firstOf(m map[string]any, keys ...string) any {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return v
		}
	}
	return nil
}
