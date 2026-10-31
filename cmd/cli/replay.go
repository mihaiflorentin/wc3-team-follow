package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/mihaiflorentin/wc3-team-follow/container"
	"github.com/mihaiflorentin/wc3-team-follow/port/contract"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

func newReplayCmd(c *container.ServiceContainer) *cobra.Command {
	return replayCmd(func() (contract.ReplayReader, error) { return c.ReplayReader() })
}

// replayCmd prints what a replay file holds, to check the replay parser.
// The text form leaves chat lines out (only their count); --json prints
// everything parsed, chat included.
func replayCmd(resolve func() (contract.ReplayReader, error)) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "replay <file.w3g>",
		Short: "Prints what Team Follow can read from a replay file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			reader, err := resolve()
			if err != nil {
				return fmt.Errorf("replay: %w", err)
			}
			r, err := reader.ReadFile(cmd.Context(), args[0])
			if err != nil {
				return fmt.Errorf("replay: %w", err)
			}
			out := cmd.OutOrStdout()
			if asJSON {
				enc := json.NewEncoder(out)
				enc.SetIndent("", "  ")
				return enc.Encode(r)
			}
			return printReplay(out, r)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print everything parsed as JSON, chat lines included")
	return cmd
}

func printReplay(w io.Writer, r dto.Replay) error {
	names := map[int]string{}
	for _, p := range r.Players {
		names[p.ID] = playerLabel(p)
	}
	name := func(id int) string {
		if n, ok := names[id]; ok {
			return n
		}
		if id < 0 {
			return "unknown"
		}
		return fmt.Sprintf("player %d", id)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Game:      %s (%d), build %d", r.Version, r.VersionNumber, r.Build)
	if r.Post202 {
		b.WriteString(", 2.0.2+ format")
	}
	fmt.Fprintf(&b, "\nLength:    %s (time slots add up to %s)\n", r.Duration.Round(time.Second), r.GameTime.Round(time.Second))
	fmt.Fprintf(&b, "Game name: %s\nHost:      %s\nSaver:     %s\n", r.GameName, name(r.HostID), name(r.SaverID))
	fmt.Fprintf(&b, "Map:       %s by %s\n           %s\n           checksum %s", r.Map.File, r.Map.Creator, r.Map.Path, r.Map.Checksum)
	if r.Map.SHA1 != "" {
		fmt.Fprintf(&b, ", sha1 %s", r.Map.SHA1)
	}
	s := r.Settings
	fmt.Fprintf(&b, "\nSettings:  speed %d, observers %s, fixed teams %v, random races %v, random hero %v\n", s.Speed, s.Observers, s.FixedTeams, s.RandomRaces, s.RandomHero)

	b.WriteString("Players:\n")
	tw := tabwriter.NewWriter(&b, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "  slot\tid\tteam\tcolor\trace\thandicap\tplayer\tactions\tAPM\tleft")
	for _, p := range r.Players {
		kind := ""
		switch {
		case p.Observer:
			kind = " (observer)"
		case p.Computer:
			kind = fmt.Sprintf(" (AI %d)", p.AIStrength)
		}
		left := "-"
		if p.Left > 0 {
			left = p.Result + " at " + p.Left.Round(time.Second).String()
			if p.Result == "" {
				left = "at " + p.Left.Round(time.Second).String()
			}
		}
		_, _ = fmt.Fprintf(tw, "  %d\t%d\t%d\t%d\t%s\t%d%%\t%s%s\t%d (%d)\t%d APM\t%s\n",
			p.Slot, p.ID, p.Team, p.Color, p.Race, p.Handicap, playerLabel(p), kind, p.Actions, p.APMActions, p.APM, left)
	}
	_ = tw.Flush()

	fmt.Fprintf(&b, "Leaves:    %d\n", len(r.Leaves))
	for _, l := range r.Leaves {
		fmt.Fprintf(&b, "  %8s  %s: %s, %s (counter %d)\n", l.Time.Round(time.Second), name(l.PlayerID), l.Reason, l.Result, l.Counter)
	}
	modes := map[string]int{}
	for _, c := range r.Chat {
		modes[c.Mode]++
	}
	fmt.Fprintf(&b, "Chat:      %d lines", len(r.Chat))
	for _, m := range []string{"lobby", "all", "allies", "observers", "private"} {
		if modes[m] > 0 {
			fmt.Fprintf(&b, ", %s %d", m, modes[m])
		}
	}
	b.WriteString(" (use --json to see them)\n")
	switch {
	case r.Winner.Draw:
		fmt.Fprintf(&b, "Winner:    draw (%s: %s)\n", r.Winner.Confidence, r.Winner.Basis)
	case r.Winner.Team >= 0:
		fmt.Fprintf(&b, "Winner:    team %d (%s: %s)\n", r.Winner.Team, r.Winner.Confidence, r.Winner.Basis)
	default:
		fmt.Fprintf(&b, "Winner:    unknown (%s)\n", r.Winner.Basis)
	}
	for _, warn := range r.Warnings {
		fmt.Fprintf(&b, "Warning:   %s\n", warn)
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func playerLabel(p dto.ReplayPlayer) string {
	label := p.Name
	if p.BattleTag != "" && p.BattleTag != p.Name {
		label = p.BattleTag
	}
	if p.Clan != "" {
		label += " [" + p.Clan + "]"
	}
	return label
}
